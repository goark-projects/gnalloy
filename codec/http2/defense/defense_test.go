package defense

import (
	"errors"
	"testing"
	"time"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec/http2"
)

func TestMaxRstFrameDecoderRejectsRstFlood(t *testing.T) {
	now := time.Unix(10, 0)
	decoder := NewMaxRstFrameDecoder(MaxRstFrameConfig{MaxFrames: 2, Window: time.Second})
	decoder.now = func() time.Time { return now }
	exceptions := &exceptionCapture{}
	ch, err := embedded.New(decoder, exceptions)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	for i := 0; i < 2; i++ {
		if _, err := ch.WriteInbound(http2.RSTStreamFrame{StreamID: http2.StreamID(1 + i*2)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ch.WriteInbound(http2.RSTStreamFrame{StreamID: 7}); err != nil {
		t.Fatal(err)
	}
	if len(exceptions.errs) != 1 || !errors.Is(exceptions.errs[0], ErrTooManyRSTFrames) {
		t.Fatalf("errors=%v, want ErrTooManyRSTFrames", exceptions.errs)
	}
}

func TestMaxRstFrameDecoderExpiresWindow(t *testing.T) {
	now := time.Unix(10, 0)
	decoder := NewMaxRstFrameDecoder(MaxRstFrameConfig{MaxFrames: 1, Window: time.Second})
	decoder.now = func() time.Time { return now }
	ch, err := embedded.New(decoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteInbound(http2.RSTStreamFrame{StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second + time.Nanosecond)
	if _, err := ch.WriteInbound(http2.RSTStreamFrame{StreamID: 3}); err != nil {
		t.Fatal(err)
	}
	if got := decoder.Count(); got != 1 {
		t.Fatalf("count=%d, want 1", got)
	}
}

func TestControlFrameLimitEncoderRejectsUnflushedControlFrames(t *testing.T) {
	encoder := NewControlFrameLimitEncoder(ControlFrameLimitConfig{MaxQueuedFrames: 1})
	ch, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if err := ch.Channel().Write(http2.PingFrame{}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Channel().Write(http2.SettingsFrame{Ack: true}); !errors.Is(err, ErrTooManyControlFrames) {
		t.Fatalf("err=%v, want ErrTooManyControlFrames", err)
	}
	if got := encoder.PendingControlFrames(); got != 1 {
		t.Fatalf("pending=%d, want 1", got)
	}
	if err := ch.Channel().Flush(); err != nil {
		t.Fatal(err)
	}
	if got := encoder.PendingControlFrames(); got != 0 {
		t.Fatalf("pending after flush=%d, want 0", got)
	}
}

type exceptionCapture struct {
	errs []error
}

func (c *exceptionCapture) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}
