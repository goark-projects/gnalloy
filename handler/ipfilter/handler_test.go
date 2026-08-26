package ipfilter

import (
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

func TestHandlerRejectsDeniedUDPDatagram(t *testing.T) {
	deny, err := DenyCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	sink := &ipFilterSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	recorder := &ipFilterRecorder{}
	if err := ch.Pipeline().AddLast("filter", NewHandler(Config{Rules: []Rule{deny}, DefaultAccept: true, CloseOnReject: true})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	payload := ipFilterBuf(t, "drop")
	ch.Pipeline().FireChannelRead(udp.Datagram{Payload: payload, Addr: udp.Address{IP: net.IPv4(10, 1, 2, 3), Port: 9000}})

	if len(recorder.messages) != 0 {
		t.Fatalf("messages=%d, want rejected", len(recorder.messages))
	}
	if payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want released", payload.RefCnt())
	}
	if sink.closed != 1 {
		t.Fatalf("closed=%d, want close on reject", sink.closed)
	}
}

func TestHandlerAcceptsFirstMatchingRule(t *testing.T) {
	denyAll, err := DenyCIDR("0.0.0.0/0")
	if err != nil {
		t.Fatal(err)
	}
	allowLoopback := AllowIP(net.IPv4(127, 0, 0, 1))
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), &ipFilterSink{})
	recorder := &ipFilterRecorder{}
	if err := ch.Pipeline().AddLast("filter", NewHandler(Config{Rules: []Rule{allowLoopback, denyAll}})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	payload := ipFilterBuf(t, "ok")
	ch.Pipeline().FireChannelRead(udp.Datagram{Payload: payload, Addr: udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 9000}})

	if len(recorder.messages) != 1 {
		t.Fatalf("messages=%d, want accepted", len(recorder.messages))
	}
	recorder.release()
}

func BenchmarkIPFilterAllowedDatagram(b *testing.B) {
	allow, err := AllowCIDR("127.0.0.0/8")
	if err != nil {
		b.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), &ipFilterSink{})
	recorder := &ipFilterRecorder{}
	if err := ch.Pipeline().AddLast("filter", NewHandler(Config{Rules: []Rule{allow}})); err != nil {
		b.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		b.Fatal(err)
	}
	payload := ipFilterBuf(b, "ok")
	defer payload.Release()
	datagram := udp.Datagram{Payload: payload, Addr: udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 9000}}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		datagram.Payload = payload.Retain()
		ch.Pipeline().FireChannelRead(datagram)
		recorder.release()
	}
}

type ipFilterRecorder struct {
	messages []any
}

func (r *ipFilterRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	r.messages = append(r.messages, msg)
}

func (r *ipFilterRecorder) release() {
	for _, msg := range r.messages {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
	r.messages = r.messages[:0]
}

type ipFilterSink struct {
	closed int
}

func (s *ipFilterSink) Write(any) error { return nil }
func (s *ipFilterSink) Flush() error    { return nil }

func (s *ipFilterSink) Close() error {
	s.closed++
	return nil
}

func ipFilterBuf(t testing.TB, data string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(data))
	if _, err := buf.WriteBytes([]byte(data)); err != nil {
		buf.Release()
		t.Fatal(err)
	}
	return buf
}

var _ channel.OutboundSink = (*ipFilterSink)(nil)
