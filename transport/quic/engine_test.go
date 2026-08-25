package quic

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

func TestPacketHandlerRoutesInitialDatagram(t *testing.T) {
	packetBytes := encodeQUICTestPacket(t, "hello")
	collector := &quicCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("quic", NewPacketHandler(PacketHandlerConfig{Router: NewConnectionIDRouter(2)})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(udp.Datagram{Payload: packetBytes, Addr: quicUDPAddr})
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	event := collector.msgs[0].(PacketEvent)
	if !event.NewConnection || event.Conn == nil {
		t.Fatalf("event=%+v, want new connection", event)
	}
	if event.Conn.Remote.String() != quicUDPAddr.String() || event.Remote.String() != quicUDPAddr.String() {
		t.Fatalf("remote conn=%s event=%s", event.Conn.Remote, event.Remote)
	}
	if event.Packet.Type != PacketInitial || event.Packet.PacketNumber != 7 {
		t.Fatalf("packet=%+v", event.Packet)
	}
	if string(event.Packet.Payload.Bytes()) != "hello" {
		t.Fatalf("payload=%q", event.Packet.Payload.Bytes())
	}
	event.Release()
	if packetBytes.RefCnt() != 0 {
		t.Fatalf("packetBytes ref=%d, want 0", packetBytes.RefCnt())
	}
}

func TestPacketHandlerEncodesOutboundPacket(t *testing.T) {
	sink := &quicCaptureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("quic", NewPacketHandler(PacketHandlerConfig{})); err != nil {
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

func TestPacketFrameDecoderAcceptsPacketEvent(t *testing.T) {
	payload, err := EncodeFrames(buffer.NewHeapAllocator(), PingFrame{})
	if err != nil {
		t.Fatal(err)
	}
	defer payload.Release()
	conn := &Connection{Remote: quicUDPAddr, State: ConnectionStateNew}
	event := PacketEvent{
		Packet: Packet{
			Type:               PacketInitial,
			Version:            Version1,
			DestinationID:      MustConnectionID([]byte{1}),
			SourceID:           MustConnectionID([]byte{2}),
			PacketNumberLength: 1,
			PacketNumber:       9,
			Payload:            payload.Retain(),
		},
		Conn:          conn,
		Remote:        quicUDPAddr,
		NewConnection: true,
	}
	collector := &quicCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("frames", NewPacketFrameDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(event)
	if len(collector.msgs) != 1 {
		t.Fatalf("frames=%d, want 1", len(collector.msgs))
	}
	addressed := collector.msgs[0].(udp.Addressed)
	frame := addressed.Message.(FrameEvent)
	if frame.Conn != conn || !frame.NewConnection || frame.Remote.String() != quicUDPAddr.String() {
		t.Fatalf("frame event=%+v", frame)
	}
	addressed.Release()
	if payload.RefCnt() != 1 {
		t.Fatalf("payload ref=%d, want caller ref only", payload.RefCnt())
	}
}

func encodeQUICTestPacket(t *testing.T, text string) buffer.ByteBuf {
	t.Helper()
	payload := quicTestBuf(text)
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
	return packetBytes
}
