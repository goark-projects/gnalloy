package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestConnectionControllerEnforcesReceiveConnectionWindow(t *testing.T) {
	controller, err := NewConnectionController(ConnectionControllerConfig{
		Server:                  true,
		InitialConnectionWindow: 4,
		InitialStreamWindow:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &connectionErrorRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	_ = ch.Pipeline().AddLast("connection", controller)
	_ = ch.Pipeline().AddLast("recorder", recorder)

	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "h")})
	ch.Pipeline().FireChannelRead(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "12345")})

	if len(recorder.errs) != 1 || !errors.Is(recorder.errs[0], ErrFlowControl) {
		t.Fatalf("errs=%v, want flow control", recorder.errs)
	}
	if controller.ConnectionReceiveWindow() != 4 || controller.StreamReceiveWindow(1) != 8 {
		t.Fatalf("windows conn=%d stream=%d, want unchanged 4/8", controller.ConnectionReceiveWindow(), controller.StreamReceiveWindow(1))
	}
}

func TestConnectionControllerAppliesReceiveWindowUpdate(t *testing.T) {
	controller, err := NewConnectionController(ConnectionControllerConfig{
		Server:                  true,
		InitialConnectionWindow: 4,
		InitialStreamWindow:     4,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &connectionErrorRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	_ = ch.Pipeline().AddLast("connection", controller)
	_ = ch.Pipeline().AddLast("recorder", recorder)

	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "h")})
	ch.Pipeline().FireChannelRead(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "abcd")})
	if controller.ConnectionReceiveWindow() != 0 || controller.StreamReceiveWindow(1) != 0 {
		t.Fatalf("windows conn=%d stream=%d, want 0/0", controller.ConnectionReceiveWindow(), controller.StreamReceiveWindow(1))
	}

	if err := ch.Write(WindowUpdateFrame{StreamID: 0, Increment: 4}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(WindowUpdateFrame{StreamID: 1, Increment: 4}); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "xy")})

	if len(recorder.errs) != 0 {
		t.Fatalf("errs=%v, want none", recorder.errs)
	}
	if controller.ConnectionReceiveWindow() != 2 || controller.StreamReceiveWindow(1) != 2 {
		t.Fatalf("windows conn=%d stream=%d, want 2/2", controller.ConnectionReceiveWindow(), controller.StreamReceiveWindow(1))
	}
}

func TestConnectionControllerEnforcesMaxConcurrentRemoteStreams(t *testing.T) {
	controller, err := NewConnectionController(ConnectionControllerConfig{Server: true, MaxConcurrentStreams: 1})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &connectionErrorRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	_ = ch.Pipeline().AddLast("connection", controller)
	_ = ch.Pipeline().AddLast("recorder", recorder)

	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "one")})
	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 3, HeaderBlock: testHTTP2Buf(t, "two")})

	if len(recorder.errs) != 1 || !errors.Is(recorder.errs[0], ErrInvalidStreamState) {
		t.Fatalf("errs=%v, want invalid stream state", recorder.errs)
	}
	if controller.ActiveStreams() != 1 {
		t.Fatalf("active=%d, want 1", controller.ActiveStreams())
	}
}

func TestConnectionControllerAppliesAndValidatesSettings(t *testing.T) {
	controller, err := NewConnectionController(ConnectionControllerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &connectionErrorRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	_ = ch.Pipeline().AddLast("connection", controller)
	_ = ch.Pipeline().AddLast("recorder", recorder)

	ch.Pipeline().FireChannelRead(SettingsFrame{Settings: []Setting{
		{ID: SettingEnablePush, Value: 0},
		{ID: SettingInitialWindowSize, Value: 1024},
		{ID: SettingMaxFrameSize, Value: DefaultMaxFrameSize * 2},
	}})
	settings := controller.RemoteSettings()
	if settings.EnablePush || settings.InitialWindowSize != 1024 || settings.MaxFrameSize != DefaultMaxFrameSize*2 {
		t.Fatalf("settings=%+v", settings)
	}

	ch.Pipeline().FireChannelRead(SettingsFrame{Settings: []Setting{{ID: SettingEnablePush, Value: 2}}})
	if len(recorder.errs) != 1 || !errors.Is(recorder.errs[0], ErrInvalidFrame) {
		t.Fatalf("errs=%v, want invalid frame", recorder.errs)
	}
}

func TestConnectionControllerRejectsPushWhenPeerDisabled(t *testing.T) {
	controller, err := NewConnectionController(ConnectionControllerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	_ = ch.Pipeline().AddLast("connection", controller)

	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "request")})
	ch.Pipeline().FireChannelRead(SettingsFrame{Settings: []Setting{{ID: SettingEnablePush, Value: 0}}})

	block := testHTTP2Buf(t, "push")
	err = ch.Write(PushPromiseFrame{StreamID: 1, PromisedStreamID: 2, HeaderBlock: block})
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("err=%v, want invalid frame", err)
	}
	if block.RefCnt() != 0 {
		t.Fatalf("block ref=%d, want released on write rejection", block.RefCnt())
	}
}

func TestConnectionControllerRejectsNewLocalStreamAfterGoAway(t *testing.T) {
	controller, err := NewConnectionController(ConnectionControllerConfig{Server: false})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	_ = ch.Pipeline().AddLast("connection", controller)

	if err := ch.Write(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "first")}); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(GoAwayFrame{LastStreamID: 1})
	if err := ch.Write(HeadersFrame{StreamID: 3, HeaderBlock: testHTTP2Buf(t, "late")}); !errors.Is(err, ErrInvalidStreamState) {
		t.Fatalf("err=%v, want invalid stream state", err)
	}
}

type connectionErrorRecorder struct {
	errs []error
}

func (r *connectionErrorRecorder) ExceptionCaught(_ *channel.HandlerContext, err error) {
	r.errs = append(r.errs, err)
}
