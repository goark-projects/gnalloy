package udp

import (
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport"
)

var testAddr = Address{IP: net.IPv4(127, 0, 0, 1), Port: 9000}

func TestParseAddressAndString(t *testing.T) {
	addr, err := parseAddress("127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	if got := addr.String(); got != "127.0.0.1:9000" {
		t.Fatalf("addr=%s", got)
	}

	ipv6, err := parseAddress("[::1%1]:9001")
	if err != nil {
		t.Fatal(err)
	}
	if !ipv6.ipv6 || ipv6.zone != "1" || ipv6.String() != "[::1%1]:9001" {
		t.Fatalf("ipv6=%+v string=%s", ipv6, ipv6.String())
	}

	if _, err := parseAddress("bad-address"); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidAddress)
	}
}

func TestDatagramToMessageDecoderPreservesAddressAndSliceLifetime(t *testing.T) {
	collector := &udpCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder := NewDatagramToMessageDecoderFunc(nil, func(_ *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
		frame, err := payload.Slice(payload.ReaderIndex(), payload.ReadableBytes())
		if err != nil {
			return err
		}
		out.Add(frame)
		return nil
	})
	if err := ch.Pipeline().AddLast("datagramDecoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	in := newUDPTestBuf("ping")
	ch.Pipeline().FireChannelRead(Datagram{Payload: in, Addr: testAddr})

	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	addressed, ok := collector.msgs[0].(Addressed)
	if !ok {
		t.Fatalf("msg=%T, want Addressed", collector.msgs[0])
	}
	if addressed.Addr.String() != testAddr.String() {
		t.Fatalf("addr=%s, want %s", addressed.Addr.String(), testAddr.String())
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

func TestDatagramReleaseAndValid(t *testing.T) {
	buf := newUDPTestBuf("ok")
	datagram := Datagram{Payload: buf, Addr: testAddr}
	if !datagram.Valid() {
		t.Fatal("datagram should be valid")
	}
	datagram.Release()
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}

	invalid := Datagram{Payload: newUDPTestBuf("bad"), Addr: Address{IP: testAddr.IP, Port: 65536}}
	defer invalid.Release()
	if invalid.Valid() {
		t.Fatal("datagram with invalid port should be rejected")
	}
}

func TestMessageToDatagramEncoderWritesAddressedPayload(t *testing.T) {
	sink := &udpCaptureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	encoder := NewMessageToDatagramEncoderFunc(func(msg any) bool {
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
	if err := ch.Pipeline().AddLast("datagramEncoder", encoder); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(Addressed{Message: "pong", Addr: testAddr}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	datagram, ok := sink.writes[0].(Datagram)
	if !ok {
		t.Fatalf("write=%T, want Datagram", sink.writes[0])
	}
	if datagram.Addr.String() != testAddr.String() {
		t.Fatalf("addr=%s, want %s", datagram.Addr.String(), testAddr.String())
	}
	if string(datagram.Payload.Bytes()) != "pong" {
		t.Fatalf("payload=%q", datagram.Payload.Bytes())
	}
}

func TestMessageToDatagramEncoderReleasesPayloadOnWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	sink := &udpCaptureSink{err: wantErr}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	encoder := NewMessageToDatagramEncoderFunc(func(any) bool { return false }, nil)
	if err := ch.Pipeline().AddLast("datagramEncoder", encoder); err != nil {
		t.Fatal(err)
	}

	buf := newUDPTestBuf("boom")
	err := ch.Write(Addressed{Message: buf, Addr: testAddr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want %v", err, wantErr)
	}
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
}

func TestEndpointReleasesCompletionBufferAfterClose(t *testing.T) {
	buf := newUDPTestBuf("late")
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
	recorder := &udpWritabilityRecorder{}
	if err := ch.Pipeline().AddLast("writable", recorder); err != nil {
		t.Fatal(err)
	}

	first := newUDPTestBuf("abc")
	second := newUDPTestBuf("de")
	ep.enqueue(Datagram{Payload: first, Addr: testAddr})
	if !ch.IsWritable() || ch.PendingOutboundBytes() != 3 || recorder.changes != 0 {
		t.Fatalf("after first writable=%v pending=%d changes=%d", ch.IsWritable(), ch.PendingOutboundBytes(), recorder.changes)
	}

	ep.enqueue(Datagram{Payload: second, Addr: testAddr})
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

type udpCaptureInbound struct {
	msgs []any
}

func (h *udpCaptureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type udpCaptureSink struct {
	writes []any
	err    error
}

func (s *udpCaptureSink) Write(msg any) error {
	if s.err != nil {
		return s.err
	}
	s.writes = append(s.writes, msg)
	return nil
}

func (s *udpCaptureSink) Flush() error {
	return nil
}

func (s *udpCaptureSink) Close() error {
	return nil
}

func (s *udpCaptureSink) release() {
	for _, msg := range s.writes {
		releaseMessage(msg)
	}
	s.writes = nil
}

func newUDPTestBuf(s string) *buffer.DirectByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}

type udpWritabilityRecorder struct {
	changes int
}

func (r *udpWritabilityRecorder) ChannelWritabilityChanged(*channel.HandlerContext) {
	r.changes++
}
