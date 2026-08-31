package protobuf

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type protoCollector struct {
	messages []proto.Message
}

func (c *protoCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if pm, ok := msg.(proto.Message); ok {
		c.messages = append(c.messages, pm)
	}
}

func TestEncoderMarshalsProtoMessage(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("protobuf-encoder", NewEncoder())
	defer sink.release()

	if err := ch.Write(wrapperspb.String("hello")); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	got := &wrapperspb.StringValue{}
	if err := proto.Unmarshal(sink.writes[0].Bytes(), got); err != nil {
		t.Fatal(err)
	}
	if got.Value != "hello" {
		t.Fatalf("value=%q, want hello", got.Value)
	}
}

func TestEncoderPreservesEmptyMessage(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("protobuf-encoder", NewEncoder())
	defer sink.release()

	if err := ch.Write(&emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	if got := sink.writes[0].ReadableBytes(); got != 0 {
		t.Fatalf("readable=%d, want 0", got)
	}
}

func TestDecoderUnmarshalsProtoMessage(t *testing.T) {
	data, err := proto.Marshal(wrapperspb.Int32(42))
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(func() proto.Message { return &wrapperspb.Int32Value{} }, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &protoCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("protobuf-decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf(data))
	if len(collector.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(collector.messages))
	}
	got := collector.messages[0].(*wrapperspb.Int32Value)
	if got.Value != 42 {
		t.Fatalf("value=%d, want 42", got.Value)
	}
}

func TestDecoderCopiesFragmentedByteBuf(t *testing.T) {
	data, err := proto.Marshal(wrapperspb.String("fragmented"))
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(func() proto.Message { return &wrapperspb.StringValue{} }, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &protoCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("protobuf-decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	composite := buffer.NewCompositeByteBuf()
	composite.Append(testBuf(data[:1]))
	composite.Append(testBuf(data[1:]))
	ch.Pipeline().FireChannelRead(composite)

	if len(collector.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(collector.messages))
	}
	got := collector.messages[0].(*wrapperspb.StringValue)
	if got.Value != "fragmented" {
		t.Fatalf("value=%q, want fragmented", got.Value)
	}
}

func TestDecoderReportsTooLongMessage(t *testing.T) {
	data, err := proto.Marshal(wrapperspb.String("too-long"))
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(func() proto.Message { return &wrapperspb.StringValue{} }, 1)
	if err != nil {
		t.Fatal(err)
	}
	collector := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("protobuf-decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf(data))
	if len(collector.errs) != 1 || !errors.Is(collector.errs[0], codec.ErrFrameTooLong) {
		t.Fatalf("errs=%v, want frame too long", collector.errs)
	}
}

func TestDecoderRejectsInvalidConfig(t *testing.T) {
	if _, err := NewDecoder(nil, 1024); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil factory err=%v, want %v", err, ErrInvalidConfig)
	}
	if _, err := NewDecoder(func() proto.Message { return &wrapperspb.StringValue{} }, -1); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("negative size err=%v, want %v", err, ErrInvalidConfig)
	}
}
