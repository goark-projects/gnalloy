package ip

import (
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport/raw"
)

var (
	ipv4Src = net.IPv4(192, 0, 2, 1)
	ipv4Dst = net.IPv4(192, 0, 2, 2)
	ipv6Src = net.ParseIP("2001:db8::1")
	ipv6Dst = net.ParseIP("2001:db8::2")
)

func TestEncodeDecodeIPv4PacketZeroCopyPayload(t *testing.T) {
	payload := testBuf("abc")
	packet := Packet{
		Header: Header{
			Version:        Version4,
			Identification: 0x1234,
			Protocol:       ProtocolICMP,
			Source:         ipv4Src,
			Destination:    ipv4Dst,
		},
		Payload: payload,
	}
	encoded, err := EncodePacket(buffer.NewHeapAllocator(), packet)
	if err != nil {
		t.Fatal(err)
	}
	defer encoded.Release()
	if encoded.Bytes()[0]>>4 != Version4 || Checksum(encoded.Bytes()[:ipv4HeaderLength]) != 0 {
		t.Fatalf("invalid ipv4 header: %v", encoded.Bytes()[:ipv4HeaderLength])
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Header.Protocol != ProtocolICMP || decoded.Header.TotalLength != ipv4HeaderLength+3 {
		t.Fatalf("header=%+v", decoded.Header)
	}
	if string(decoded.Payload.Bytes()) != "abc" {
		t.Fatalf("payload=%q", decoded.Payload.Bytes())
	}
	if encoded.RefCnt() != 2 {
		t.Fatalf("encoded ref=%d, want 2 while payload slice is alive", encoded.RefCnt())
	}
	decoded.Release()
	if encoded.RefCnt() != 1 {
		t.Fatalf("encoded ref=%d, want 1 after release", encoded.RefCnt())
	}
}

func TestEncodeDecodeIPv6Packet(t *testing.T) {
	payload := testBuf("hello")
	packet := Packet{
		Header: Header{
			Version:      Version6,
			TrafficClass: 7,
			FlowLabel:    0xabcde,
			NextHeader:   ProtocolUDP,
			Source:       ipv6Src,
			Destination:  ipv6Dst,
		},
		Payload: payload,
	}
	encoded, err := EncodePacket(buffer.NewHeapAllocator(), packet)
	if err != nil {
		t.Fatal(err)
	}
	defer encoded.Release()
	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Release()
	if decoded.Header.Version != Version6 || decoded.Header.NextHeader != ProtocolUDP {
		t.Fatalf("header=%+v", decoded.Header)
	}
	if decoded.Header.PayloadLength != 5 || string(decoded.Payload.Bytes()) != "hello" {
		t.Fatalf("decoded=%+v payload=%q", decoded.Header, decoded.Payload.Bytes())
	}
}

func TestPipelineDecodeEncodeRawAddressed(t *testing.T) {
	encoded, err := EncodePacket(buffer.NewHeapAllocator(), Packet{
		Header:  Header{Version: Version4, Protocol: ProtocolICMP, Source: ipv4Src, Destination: ipv4Dst},
		Payload: testBuf("x"),
	})
	if err != nil {
		t.Fatal(err)
	}

	collector := &ipCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("ipDecoder", NewDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(raw.Addressed{Message: encoded, Addr: raw.Address{IP: ipv4Src}, Protocol: raw.ProtocolRaw})
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	addressed := collector.msgs[0].(raw.Addressed)
	if addressed.Protocol != ProtocolICMP {
		t.Fatalf("protocol=%d, want %d", addressed.Protocol, ProtocolICMP)
	}
	decoded := addressed.Message.(Packet)
	addressed.Release()
	if decoded.Payload != nil && decoded.Payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want released", decoded.Payload.RefCnt())
	}

	sink := &ipCaptureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("ipEncoder", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	err = outCh.Write(raw.Addressed{
		Message: Packet{
			Header:  Header{Version: Version4, Protocol: ProtocolICMP, Source: ipv4Src, Destination: ipv4Dst},
			Payload: testBuf("y"),
		},
		Addr:     raw.Address{IP: ipv4Dst},
		Protocol: raw.ProtocolRaw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	out := sink.writes[0].(raw.Addressed)
	if _, ok := out.Message.(buffer.ByteBuf); !ok {
		t.Fatalf("message=%T, want ByteBuf", out.Message)
	}
}

func TestProtocolPayloadDecoderDispatchesCustomProtocol(t *testing.T) {
	collector := &ipCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder := NewProtocolPayloadDecoderFunc(func(protocol int) bool {
		return protocol == 253
	}, func(_ *channel.HandlerContext, _ Header, payload buffer.ByteBuf, out *codec.MessageList) error {
		frame, err := payload.Slice(payload.ReaderIndex(), payload.ReadableBytes())
		if err != nil {
			return err
		}
		out.Add(frame)
		return nil
	})
	if err := ch.Pipeline().AddLast("customDecoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	payload := testBuf("custom")
	ch.Pipeline().FireChannelRead(raw.Addressed{
		Message:  Packet{Header: Header{Version: Version4, Protocol: 253}, Payload: payload},
		Addr:     raw.Address{IP: ipv4Src},
		Protocol: raw.ProtocolRaw,
	})
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	addressed := collector.msgs[0].(raw.Addressed)
	if addressed.Protocol != 253 {
		t.Fatalf("protocol=%d, want 253", addressed.Protocol)
	}
	frame := addressed.Message.(buffer.ByteBuf)
	if string(frame.Bytes()) != "custom" {
		t.Fatalf("payload=%q", frame.Bytes())
	}
	if payload.RefCnt() != 1 {
		t.Fatalf("payload ref=%d, want 1 while dispatched slice is alive", payload.RefCnt())
	}
	addressed.Release()
	if payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want 0 after release", payload.RefCnt())
	}
}

func TestProtocolPayloadEncoderBuildsRawIPPackedPayload(t *testing.T) {
	sink := &ipCaptureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("ipEncoder", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	encoder := NewProtocolPayloadEncoderFunc(
		ProtocolPayloadEncoderConfig{Version: Version4, Source: ipv4Src},
		func(msg any) bool {
			_, ok := msg.(string)
			return ok
		},
		func(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error {
			text := msg.(string)
			buf, err := ctx.Channel().Allocator().Acquire(len(text))
			if err != nil {
				return err
			}
			if _, err := buf.WriteBytes([]byte(text)); err != nil {
				buf.Release()
				return err
			}
			out.Add(buf)
			return nil
		},
	)
	if err := ch.Pipeline().AddLast("customEncoder", encoder); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(raw.Addressed{Message: "custom", Addr: raw.Address{IP: ipv4Dst}, Protocol: 253}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	out := sink.writes[0].(raw.Addressed)
	encoded := out.Message.(buffer.ByteBuf)
	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Release()
	if decoded.Header.Protocol != 253 || !decoded.Header.Source.Equal(ipv4Src.To4()) || !decoded.Header.Destination.Equal(ipv4Dst.To4()) {
		t.Fatalf("header=%+v", decoded.Header)
	}
	if string(decoded.Payload.Bytes()) != "custom" {
		t.Fatalf("payload=%q", decoded.Payload.Bytes())
	}
}

type ipCaptureInbound struct {
	msgs []any
}

func (h *ipCaptureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type ipCaptureSink struct {
	writes []any
}

func (s *ipCaptureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *ipCaptureSink) Flush() error {
	return nil
}

func (s *ipCaptureSink) Close() error {
	return nil
}

func (s *ipCaptureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

func testBuf(s string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}
