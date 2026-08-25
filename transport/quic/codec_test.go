package quic

import (
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

var quicUDPAddr = udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433}

func TestDatagramToPacketDecoderPreservesAddressAndPayloadSlice(t *testing.T) {
	payload := quicTestBuf("hello")
	header := Header{
		Type:               PacketInitial,
		Version:            Version1,
		DestinationID:      MustConnectionID([]byte{1, 2}),
		SourceID:           MustConnectionID([]byte{3}),
		PacketNumberLength: 1,
		PacketNumber:       7,
		Length:             uint64(1 + payload.ReadableBytes()),
	}
	encoded, err := AppendHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}
	packetBytes := buffer.NewHeapBuffer(len(encoded) + payload.ReadableBytes())
	_, _ = packetBytes.WriteBytes(encoded)
	_, _ = packetBytes.WriteBytes(payload.Bytes())
	payload.Release()

	collector := &quicCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("quic", NewDatagramToPacketDecoder(DatagramToPacketDecoderConfig{})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(udp.Datagram{Payload: packetBytes, Addr: quicUDPAddr})
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	addressed := collector.msgs[0].(udp.Addressed)
	packet := addressed.Message.(Packet)
	if addressed.Addr.String() != quicUDPAddr.String() {
		t.Fatalf("addr=%s, want %s", addressed.Addr, quicUDPAddr)
	}
	if packet.Type != PacketInitial || packet.PacketNumber != 7 {
		t.Fatalf("packet=%+v", packet)
	}
	if string(packet.Payload.Bytes()) != "hello" {
		t.Fatalf("payload=%q", packet.Payload.Bytes())
	}
	if packetBytes.RefCnt() != 1 {
		t.Fatalf("packetBytes ref=%d, want 1 while payload slice is alive", packetBytes.RefCnt())
	}
	addressed.Release()
	if packetBytes.RefCnt() != 0 {
		t.Fatalf("packetBytes ref=%d, want 0 after release", packetBytes.RefCnt())
	}
}

func TestPacketToDatagramEncoderBuildsUDPDatagram(t *testing.T) {
	sink := &quicCaptureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("quic", NewPacketToDatagramEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	payload := quicTestBuf("ok")
	packet := Packet{
		Type:               PacketHandshake,
		Version:            Version1,
		DestinationID:      MustConnectionID([]byte{9}),
		SourceID:           MustConnectionID([]byte{8}),
		PacketNumberLength: 2,
		PacketNumber:       0x1234,
		Payload:            payload,
	}
	if err := ch.Write(udp.Addressed{Message: packet, Addr: quicUDPAddr}); err != nil {
		t.Fatal(err)
	}
	if payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want 0 after encode", payload.RefCnt())
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	datagram := sink.writes[0].(udp.Datagram)
	decoded, err := DecodePacket(datagram.Payload, HeaderParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Release()
	if decoded.Type != PacketHandshake || decoded.PacketNumber != 0x1234 {
		t.Fatalf("decoded=%+v", decoded)
	}
	if string(decoded.Payload.Bytes()) != "ok" {
		t.Fatalf("payload=%q", decoded.Payload.Bytes())
	}
}

type quicCaptureInbound struct {
	msgs []any
}

func (h *quicCaptureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type quicCaptureSink struct {
	writes []any
}

func (s *quicCaptureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *quicCaptureSink) Flush() error {
	return nil
}

func (s *quicCaptureSink) Close() error {
	return nil
}

func (s *quicCaptureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

func quicTestBuf(s string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}
