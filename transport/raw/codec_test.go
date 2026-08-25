package raw

import (
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport"
)

var testAddr = Address{IP: net.IPv4(127, 0, 0, 1)}

func TestParseAddressAndString(t *testing.T) {
	addr, err := parseAddress("127.0.0.1", FamilyIPv4)
	if err != nil {
		t.Fatal(err)
	}
	if got := addr.String(); got != "127.0.0.1" {
		t.Fatalf("addr=%s", got)
	}

	ipv6, err := parseAddress("::1%1", FamilyIPv6)
	if err != nil {
		t.Fatal(err)
	}
	if !ipv6.ipv6 || ipv6.zone != "1" || ipv6.String() != "::1%1" {
		t.Fatalf("ipv6=%+v string=%s", ipv6, ipv6.String())
	}

	if _, err := parseAddress("::1", FamilyIPv4); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidAddress)
	}
}

func TestPacketToMessageDecoderPreservesAddressProtocolAndSliceLifetime(t *testing.T) {
	collector := &rawCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder := NewPacketToMessageDecoderFunc(nil, func(_ *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
		frame, err := payload.Slice(payload.ReaderIndex(), payload.ReadableBytes())
		if err != nil {
			return err
		}
		out.Add(frame)
		return nil
	})
	if err := ch.Pipeline().AddLast("packetDecoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	in := newRawTestBuf("ping")
	ch.Pipeline().FireChannelRead(Packet{Payload: in, Addr: testAddr, Protocol: ProtocolICMP})

	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	addressed, ok := collector.msgs[0].(Addressed)
	if !ok {
		t.Fatalf("msg=%T, want Addressed", collector.msgs[0])
	}
	if addressed.Addr.String() != testAddr.String() || addressed.Protocol != ProtocolICMP {
		t.Fatalf("addressed=%+v", addressed)
	}
	frame, ok := addressed.Message.(buffer.ByteBuf)
	if !ok {
		t.Fatalf("message=%T, want ByteBuf", addressed.Message)
	}
	if string(frame.Bytes()) != "ping" {
		t.Fatalf("payload=%q", frame.Bytes())
	}
	if in.RefCnt() != 1 {
		t.Fatalf("input ref=%d, want 1 while slice is alive", in.RefCnt())
	}
	addressed.Release()
	if in.RefCnt() != 0 {
		t.Fatalf("input ref=%d, want 0 after addressed release", in.RefCnt())
	}
}

func TestPacketReleaseAndValid(t *testing.T) {
	buf := newRawTestBuf("ok")
	packet := Packet{Payload: buf, Addr: testAddr, Protocol: ProtocolICMP}
	if !packet.Valid() {
		t.Fatal("packet should be valid")
	}
	packet.Release()
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}

	invalid := Packet{Payload: newRawTestBuf("bad"), Addr: testAddr, Protocol: 256}
	defer invalid.Release()
	if invalid.Valid() {
		t.Fatal("packet with invalid protocol should be rejected")
	}
}

func TestMessageToPacketEncoderWritesAddressedPayload(t *testing.T) {
	sink := &rawCaptureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	encoder := NewMessageToPacketEncoderFunc(func(msg any) bool {
		_, ok := msg.(string)
		return ok
	}, func(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error {
		payload := msg.(string)
		buf, err := ctx.Channel().Allocator().Acquire(len(payload))
		if err != nil {
			return err
		}
		if _, err := buf.WriteBytes([]byte(payload)); err != nil {
			buf.Release()
			return err
		}
		out.Add(buf)
		return nil
	})
	if err := ch.Pipeline().AddLast("packetEncoder", encoder); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(Addressed{Message: "pong", Addr: testAddr, Protocol: ProtocolICMP}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	packet, ok := sink.writes[0].(Packet)
	if !ok {
		t.Fatalf("write=%T, want Packet", sink.writes[0])
	}
	if packet.Addr.String() != testAddr.String() || packet.Protocol != ProtocolICMP {
		t.Fatalf("packet=%+v", packet)
	}
	if string(packet.Payload.Bytes()) != "pong" {
		t.Fatalf("payload=%q", packet.Payload.Bytes())
	}
}

func TestPacketEncoderWritesAddressedByteBuf(t *testing.T) {
	sink := &rawCaptureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("packetEncoder", NewPacketEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(Addressed{Message: newRawTestBuf("custom"), Addr: testAddr, Protocol: 253}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	packet, ok := sink.writes[0].(Packet)
	if !ok {
		t.Fatalf("write=%T, want Packet", sink.writes[0])
	}
	if packet.Addr.String() != testAddr.String() || packet.Protocol != 253 {
		t.Fatalf("packet=%+v", packet)
	}
	if string(packet.Payload.Bytes()) != "custom" {
		t.Fatalf("payload=%q", packet.Payload.Bytes())
	}
}

func TestMessageToPacketEncoderReleasesPayloadOnWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	sink := &rawCaptureSink{err: wantErr}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	encoder := NewMessageToPacketEncoderFunc(func(any) bool { return false }, nil)
	if err := ch.Pipeline().AddLast("packetEncoder", encoder); err != nil {
		t.Fatal(err)
	}

	buf := newRawTestBuf("boom")
	err := ch.Write(Addressed{Message: buf, Addr: testAddr, Protocol: ProtocolICMP})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want %v", err, wantErr)
	}
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
}

func TestEndpointReleasesCompletionBufferAfterClose(t *testing.T) {
	buf := newRawTestBuf("late")
	ep := &endpoint{closed: true}

	ep.HandleEvent(transport.PollEvent{
		Model: transport.PollerCompletion,
		Op:    transport.OpRead,
		Buf:   buf,
	})

	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
}

func TestEndpointBackpressureWatermark(t *testing.T) {
	ep := &endpoint{}
	ep.initBackpressure(transport.WriteBufferWatermark{Low: 2, High: 4})
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), ep)
	ep.ch = ch
	recorder := &rawWritabilityRecorder{}
	if err := ch.Pipeline().AddLast("writable", recorder); err != nil {
		t.Fatal(err)
	}

	first := newRawTestBuf("abc")
	second := newRawTestBuf("de")
	ep.enqueue(Packet{Payload: first, Addr: testAddr, Protocol: ProtocolICMP})
	if !ch.IsWritable() || ch.PendingOutboundBytes() != 3 || recorder.changes != 0 {
		t.Fatalf("after first writable=%v pending=%d changes=%d", ch.IsWritable(), ch.PendingOutboundBytes(), recorder.changes)
	}

	ep.enqueue(Packet{Payload: second, Addr: testAddr, Protocol: ProtocolICMP})
	if ch.IsWritable() || ch.PendingOutboundBytes() != 5 || recorder.changes != 1 {
		t.Fatalf("after second writable=%v pending=%d changes=%d", ch.IsWritable(), ch.PendingOutboundBytes(), recorder.changes)
	}

	ep.dequeue()
	if !ch.IsWritable() || ch.PendingOutboundBytes() != 2 || recorder.changes != 2 {
		t.Fatalf("after dequeue writable=%v pending=%d changes=%d", ch.IsWritable(), ch.PendingOutboundBytes(), recorder.changes)
	}
	ep.releaseOutbound()
	if first.RefCnt() != 0 || second.RefCnt() != 0 {
		t.Fatalf("refs=%d,%d, want 0,0", first.RefCnt(), second.RefCnt())
	}
}

type rawCaptureInbound struct {
	msgs []any
}

func (h *rawCaptureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type rawCaptureSink struct {
	writes []any
	err    error
}

func (s *rawCaptureSink) Write(msg any) error {
	if s.err != nil {
		return s.err
	}
	s.writes = append(s.writes, msg)
	return nil
}

func (s *rawCaptureSink) Flush() error {
	return nil
}

func (s *rawCaptureSink) Close() error {
	return nil
}

func (s *rawCaptureSink) release() {
	for _, msg := range s.writes {
		releaseMessage(msg)
	}
	s.writes = nil
}

func newRawTestBuf(s string) *buffer.DirectByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}

type rawWritabilityRecorder struct {
	changes int
}

func (r *rawWritabilityRecorder) ChannelWritabilityChanged(*channel.HandlerContext) {
	r.changes++
}
