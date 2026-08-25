package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestFrameDecoderDecodesPayloadZeroCopy(t *testing.T) {
	decoder, err := NewFrameDecoder(DefaultMaxFrameSize)
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("h2", decoder); err != nil {
		t.Fatal(err)
	}
	recorder := &frameRecorder{}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	in, err := ch.Allocator().Acquire(FrameHeaderSize + 4)
	if err != nil {
		t.Fatal(err)
	}
	appendRawFrame(t, in, FrameHeader{Length: 4, Type: FrameData, Flags: FlagEndStream, StreamID: 1}, []byte("ping"))
	ch.Pipeline().FireChannelRead(in)

	frame, ok := recorder.msg.(Frame)
	if !ok {
		t.Fatalf("msg=%T, want Frame", recorder.msg)
	}
	defer frame.Release()
	if frame.Type != FrameData || frame.Flags != FlagEndStream || frame.StreamID != 1 {
		t.Fatalf("frame=%+v", frame)
	}
	if string(frame.Payload.Bytes()) != "ping" {
		t.Fatalf("payload=%q, want ping", frame.Payload.Bytes())
	}
}

func TestFrameEncoderWritesHeaderThenPayload(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("h2", NewFrameEncoder()); err != nil {
		t.Fatal(err)
	}
	payload, err := ch.Allocator().Acquire(4)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = payload.WriteBytes([]byte("pong"))
	if err := ch.Write(Frame{Type: FrameData, Flags: FlagEndStream, StreamID: 3, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if len(sink.messages) != 2 {
		t.Fatalf("writes=%d, want header and payload", len(sink.messages))
	}
	header := sink.messages[0].(buffer.ByteBuf)
	defer header.Release()
	body := sink.messages[1].(buffer.ByteBuf)
	defer body.Release()
	decoded, err := ParseFrameHeader(header.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Length != 4 || decoded.Type != FrameData || decoded.Flags != FlagEndStream || decoded.StreamID != 3 {
		t.Fatalf("header=%+v", decoded)
	}
	if string(body.Bytes()) != "pong" {
		t.Fatalf("body=%q, want pong", body.Bytes())
	}
}

func TestSettingsAckHasNoPayload(t *testing.T) {
	frame := SettingsAck()
	if frame.Type != FrameSettings || frame.Flags != FlagAck || frame.StreamID != 0 || frame.Payload != nil {
		t.Fatalf("settings ack=%+v", frame)
	}
}

func TestStreamStateTransitions(t *testing.T) {
	s := NewStream(1)
	if !s.ID.ClientInitiated() || s.State != StreamIdle {
		t.Fatalf("stream=%+v", s)
	}
	if err := s.Open(false); err != nil {
		t.Fatal(err)
	}
	if err := s.HalfCloseRemote(); err != nil {
		t.Fatal(err)
	}
	if s.State != StreamHalfClosedRemote {
		t.Fatalf("state=%v, want half-closed remote", s.State)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Open(false); !errors.Is(err, ErrInvalidStreamState) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidStreamState)
	}
}

func appendRawFrame(t *testing.T, out buffer.ByteBuf, header FrameHeader, payload []byte) {
	t.Helper()
	raw, err := AppendFrameHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, payload...)
	if _, err := out.WriteBytes(raw); err != nil {
		t.Fatal(err)
	}
}

type captureSink struct {
	messages []any
}

func (s *captureSink) Write(msg any) error {
	s.messages = append(s.messages, msg)
	return nil
}

func (s *captureSink) Flush() error { return nil }
func (s *captureSink) Close() error { return nil }

type frameRecorder struct {
	msg any
}

func (r *frameRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	r.msg = msg
}
