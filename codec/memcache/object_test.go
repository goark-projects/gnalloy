package memcache

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

func TestObjectAggregatorConvertsRequestFrame(t *testing.T) {
	aggregator, err := NewObjectAggregator(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("aggregator", aggregator)
	_ = ch.Pipeline().AddLast("collector", collector)

	key := testBuf([]byte("key"))
	value := testBuf([]byte("value"))
	frame := NewRequest(OpcodeSet, nil, key, value)
	frame.Opaque = 7
	ch.Pipeline().FireChannelRead(frame)

	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	request := collector.msgs[0].(Request)
	defer request.Release()
	if request.Opcode != OpcodeSet || request.Opaque != 7 {
		t.Fatalf("request=%+v", request)
	}
	if string(request.Key.Bytes()) != "key" || string(request.Value.Bytes()) != "value" {
		t.Fatalf("key=%q value=%q", request.Key.Bytes(), request.Value.Bytes())
	}
	if key.RefCnt() != 1 || value.RefCnt() != 1 {
		t.Fatalf("key ref=%d value ref=%d, want retained object ownership", key.RefCnt(), value.RefCnt())
	}
}

func TestObjectAggregatorReportsTooLongBody(t *testing.T) {
	aggregator, err := NewObjectAggregator(1)
	if err != nil {
		t.Fatal(err)
	}
	collector := &objectErrorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("aggregator", aggregator)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(NewRequest(OpcodeSet, nil, testBuf([]byte("key")), nil))
	if len(collector.errs) != 1 || !errors.Is(collector.errs[0], codec.ErrFrameTooLong) {
		t.Fatalf("errs=%v, want frame too long", collector.errs)
	}
}

func TestClientCodecOutboundRequestEncodesFrame(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := AddClientCodec(ch.Pipeline(), 1024); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	request := NewFullRequest(OpcodeGet, nil, testBuf([]byte("key")), nil)
	request.Opaque = 0x01020304
	if err := ch.Write(request); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	header := sink.writes[0].(buffer.ByteBuf).Bytes()
	if header[0] != MagicRequest || header[1] != byte(OpcodeGet) || header[2] != 0 || header[3] != 3 {
		t.Fatalf("header=%v", header)
	}
	if got := string(sink.writes[1].(buffer.ByteBuf).Bytes()); got != "key" {
		t.Fatalf("key=%q", got)
	}
}

func TestClientCodecInboundResponseDecodesObject(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := AddClientCodec(ch.Pipeline(), 1024); err != nil {
		t.Fatal(err)
	}
	_ = ch.Pipeline().AddLast("collector", collector)

	wire := []byte{
		MagicResponse, byte(OpcodeGet), 0, 3, 0, 0, 0, byte(StatusOK),
		0, 0, 0, 8, 0, 0, 0, 9,
		0, 0, 0, 0, 0, 0, 0, 1,
		'k', 'e', 'y', 'v', 'a', 'l', 'u', 'e',
	}
	ch.Pipeline().FireChannelRead(testBuf(wire))

	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	response := collector.msgs[0].(Response)
	defer response.Release()
	if response.Status != StatusOK || response.Opaque != 9 || response.CAS != 1 {
		t.Fatalf("response=%+v", response)
	}
	if string(response.Key.Bytes()) != "key" || string(response.Value.Bytes()) != "value" {
		t.Fatalf("key=%q value=%q", response.Key.Bytes(), response.Value.Bytes())
	}
}

func TestServerCodecInboundRequestAndOutboundResponse(t *testing.T) {
	collector := &captureInbound{}
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := AddServerCodec(ch.Pipeline(), 1024); err != nil {
		t.Fatal(err)
	}
	_ = ch.Pipeline().AddLast("collector", collector)
	defer sink.release()

	wire := []byte{
		MagicRequest, byte(OpcodeGet), 0, 3, 0, 0, 0, 2,
		0, 0, 0, 3, 0, 0, 0, 7,
		0, 0, 0, 0, 0, 0, 0, 0,
		'k', 'e', 'y',
	}
	ch.Pipeline().FireChannelRead(testBuf(wire))
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	request := collector.msgs[0].(Request)
	defer request.Release()
	if request.VBucket != 2 || request.Opaque != 7 || string(request.Key.Bytes()) != "key" {
		t.Fatalf("request=%+v key=%q", request, request.Key.Bytes())
	}

	response := NewFullResponse(OpcodeGet, StatusKeyNotFound, nil, testBuf([]byte("key")), nil)
	response.Opaque = request.Opaque
	if err := ch.Write(response); err != nil {
		t.Fatal(err)
	}
	header := sink.writes[0].(buffer.ByteBuf).Bytes()
	if header[0] != MagicResponse || header[1] != byte(OpcodeGet) || header[6] != 0 || header[7] != byte(StatusKeyNotFound) {
		t.Fatalf("header=%v", header)
	}
}

func TestAddClientCodecRejectsInvalidPipeline(t *testing.T) {
	if err := AddClientCodec(nil, 1024); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

type objectErrorCollector struct {
	errs []error
}

func (c *objectErrorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}
