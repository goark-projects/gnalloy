package codec

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestMessageToByteEncoder(t *testing.T) {
	sink := &codecOutboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	encoder := NewMessageToByteEncoderFunc(
		func(msg any) bool { _, ok := msg.(string); return ok },
		func(*channel.HandlerContext, any) int { return 4 },
		func(_ *channel.HandlerContext, msg any, out buffer.ByteBuf) error {
			_, err := out.WriteBytes([]byte(msg.(string)))
			return err
		},
	)
	_ = ch.Pipeline().AddLast("encoder", encoder)
	defer sink.release()

	if err := ch.Write("ping"); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	buf := sink.writes[0].(buffer.ByteBuf)
	if string(buf.Bytes()) != "ping" {
		t.Fatalf("buf=%q", buf.Bytes())
	}
}

func TestMessageToMessageDecoder(t *testing.T) {
	collector := &captureStringInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder := NewMessageToMessageDecoderFunc(
		func(msg any) bool { _, ok := msg.(string); return ok },
		func(_ *channel.HandlerContext, msg any, out *MessageList) error {
			out.Add(msg.(string) + "-1")
			out.Add(msg.(string) + "-2")
			return nil
		},
	)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead("m")
	if len(collector.msgs) != 2 || collector.msgs[0] != "m-1" || collector.msgs[1] != "m-2" {
		t.Fatalf("msgs=%v", collector.msgs)
	}
}

func TestMessageToMessageEncoder(t *testing.T) {
	sink := &codecOutboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	encoder := NewMessageToMessageEncoderFunc(
		func(msg any) bool { _, ok := msg.(string); return ok },
		func(_ *channel.HandlerContext, msg any, out *MessageList) error {
			out.Add(msg.(string) + "a")
			out.Add(msg.(string) + "b")
			return nil
		},
	)
	_ = ch.Pipeline().AddLast("encoder", encoder)

	if err := ch.Write("x"); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 || sink.writes[0] != "xa" || sink.writes[1] != "xb" {
		t.Fatalf("writes=%v", sink.writes)
	}
}

func TestCombinedChannelDuplexHandler(t *testing.T) {
	sink := &codecOutboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	collector := &captureStringInbound{}
	encoder := NewMessageToMessageEncoderFunc(
		func(msg any) bool { _, ok := msg.(string); return ok },
		func(_ *channel.HandlerContext, msg any, out *MessageList) error {
			out.Add("out-" + msg.(string))
			return nil
		},
	)
	combined := NewCombinedChannelDuplexHandler(collector, encoder)
	_ = ch.Pipeline().AddLast("combined", combined)

	ch.Pipeline().FireChannelRead("in")
	if len(collector.msgs) != 1 || collector.msgs[0] != "in" {
		t.Fatalf("in=%v", collector.msgs)
	}
	if err := ch.Write("msg"); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 || sink.writes[0] != "out-msg" {
		t.Fatalf("out=%v", sink.writes)
	}
}
