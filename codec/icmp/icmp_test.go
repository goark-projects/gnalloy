package icmp

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/raw"
)

var (
	testIPv4 = net.IPv4(127, 0, 0, 1)
	testIPv6 = net.ParseIP("2001:db8::2")
	srcIPv6  = net.ParseIP("2001:db8::1")
)

func TestChecksumComputesOnesComplement(t *testing.T) {
	data := []byte{TypeEchoRequest, 0, 0, 0, 0x12, 0x34, 0, 1, 'p', 'i', 'n', 'g'}
	sum := Checksum(data)
	binary.BigEndian.PutUint16(data[2:4], sum)
	if got := Checksum(data); got != 0 {
		t.Fatalf("checksum verification=%#x, want 0", got)
	}
}

func TestEncodeDecodeIPv4EchoRequest(t *testing.T) {
	alloc := buffer.NewHeapAllocator()
	payload := testBuf("hello")
	msg := NewEchoRequest(raw.ProtocolICMP, 7, 9, payload)
	out, err := Encode(alloc, msg, raw.ProtocolICMP, testIPv4, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Release()

	bytes := out.Bytes()
	if bytes[0] != TypeEchoRequest || bytes[1] != 0 {
		t.Fatalf("header=%v", bytes[:2])
	}
	if Checksum(bytes) != 0 {
		t.Fatalf("invalid checksum bytes=%v", bytes)
	}

	decoded, err := Decode(out, raw.ProtocolICMP)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Release()
	if !decoded.IsEchoRequest() || decoded.Identifier != 7 || decoded.Sequence != 9 {
		t.Fatalf("decoded=%+v", decoded)
	}
	if string(decoded.Payload.Bytes()) != "hello" {
		t.Fatalf("payload=%q", decoded.Payload.Bytes())
	}
}

func TestDecodeIPv4RawPacketWithIPHeader(t *testing.T) {
	alloc := buffer.NewHeapAllocator()
	icmpPacket, err := Encode(alloc, NewEchoReply(raw.ProtocolICMP, 12, 34, testBuf("reply")), raw.ProtocolICMP, testIPv4, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer icmpPacket.Release()

	packet := buffer.NewHeapBuffer(20 + icmpPacket.ReadableBytes())
	ipHeader := make([]byte, 20)
	ipHeader[0] = 0x45
	ipHeader[9] = raw.ProtocolICMP
	binary.BigEndian.PutUint16(ipHeader[2:4], uint16(len(ipHeader)+icmpPacket.ReadableBytes()))
	_, _ = packet.WriteBytes(ipHeader)
	_, _ = packet.WriteBytes(icmpPacket.Bytes())
	defer packet.Release()

	decoded, err := Decode(packet, raw.ProtocolICMP)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Release()
	if !decoded.IsEchoReply() || decoded.Identifier != 12 || decoded.Sequence != 34 {
		t.Fatalf("decoded=%+v", decoded)
	}
	if string(decoded.Payload.Bytes()) != "reply" {
		t.Fatalf("payload=%q", decoded.Payload.Bytes())
	}
}

func TestEncodeIPv6RequiresPseudoHeaderAndComputesChecksum(t *testing.T) {
	alloc := buffer.NewHeapAllocator()
	msg := NewEchoRequest(raw.ProtocolICMPv6, 1, 2, testBuf("v6"))
	_, err := Encode(alloc, msg, raw.ProtocolICMPv6, testIPv6, nil)
	if !errors.Is(err, ErrMissingIPv6PseudoHdr) {
		t.Fatalf("err=%v, want %v", err, ErrMissingIPv6PseudoHdr)
	}

	msg.SourceIP = srcIPv6
	out, err := Encode(alloc, msg, raw.ProtocolICMPv6, testIPv6, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Release()
	if got, err := ChecksumIPv6(srcIPv6, testIPv6, out.Bytes()); err != nil || got != 0 {
		t.Fatalf("icmpv6 checksum=%#x err=%v", got, err)
	}
}

func TestDecoderPreservesPayloadSliceLifetime(t *testing.T) {
	alloc := buffer.NewHeapAllocator()
	src := testBuf("data")
	msg := NewEchoReply(raw.ProtocolICMP, 3, 4, src)
	packet, err := Encode(alloc, msg, raw.ProtocolICMP, testIPv4, nil)
	if err != nil {
		t.Fatal(err)
	}
	src.Release()

	decoded, err := Decode(packet, raw.ProtocolICMP)
	if err != nil {
		t.Fatal(err)
	}
	if packet.RefCnt() != 2 {
		t.Fatalf("packet ref=%d, want 2 with payload slice", packet.RefCnt())
	}
	decoded.Release()
	if packet.RefCnt() != 1 {
		t.Fatalf("packet ref=%d, want 1 after decoded release", packet.RefCnt())
	}
	packet.Release()
}

func TestPipelineDecodeEncodeRawAddressed(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("icmpDecoder", NewDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	packet, err := Encode(buffer.NewHeapAllocator(), NewEchoRequest(raw.ProtocolICMP, 10, 11, testBuf("abc")), raw.ProtocolICMP, testIPv4, nil)
	if err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(raw.Addressed{Message: packet, Addr: raw.Address{IP: testIPv4}, Protocol: raw.ProtocolICMP})
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	addressed := collector.msgs[0].(raw.Addressed)
	decoded := addressed.Message.(*Message)
	if decoded.Identifier != 10 || decoded.Sequence != 11 {
		t.Fatalf("decoded=%+v", decoded)
	}
	addressed.Release()

	sink := &captureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("icmpEncoder", NewEncoder(EncoderConfig{})); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	err = outCh.Write(raw.Addressed{
		Message:  NewEchoReply(raw.ProtocolICMP, 10, 11, testBuf("abc")),
		Addr:     raw.Address{IP: testIPv4},
		Protocol: raw.ProtocolICMP,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	out := sink.writes[0].(raw.Addressed)
	if out.Protocol != raw.ProtocolICMP {
		t.Fatalf("protocol=%d", out.Protocol)
	}
	if _, ok := out.Message.(buffer.ByteBuf); !ok {
		t.Fatalf("message=%T, want ByteBuf", out.Message)
	}
}

type captureInbound struct {
	msgs []any
}

func (h *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type captureSink struct {
	writes []any
}

func (s *captureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *captureSink) Flush() error {
	return nil
}

func (s *captureSink) Close() error {
	return nil
}

func (s *captureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
	s.writes = nil
}

func testBuf(s string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}
