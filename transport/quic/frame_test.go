package quic

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

func TestFrameScannerDecodesFramesWithZeroCopyPayloads(t *testing.T) {
	cryptoPayload := quicTestBuf("abc")
	streamPayload := quicTestBuf("xyz")
	reason := quicTestBuf("bye")
	pathData := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	encoded, err := AppendFrame(nil, PaddingFrame{Length: 2})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, PingFrame{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, ACKFrame{
		LargestAcked:  10,
		Delay:         1,
		FirstAckRange: 3,
		AdditionalRanges: []ACKRange{
			{Gap: 1, Length: 2},
		},
		ECN: &ECNCounts{ECT0: 1, ECT1: 2, CE: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, CryptoFrame{Offset: 4, Data: cryptoPayload})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, StreamFrame{StreamID: 7, Offset: 9, Fin: true, Data: streamPayload})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, ConnectionCloseFrame{ErrorCode: 42, FrameType: FrameTypeCrypto, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, PathChallengeFrame{Data: pathData})
	if err != nil {
		t.Fatal(err)
	}
	cryptoPayload.Release()
	streamPayload.Release()
	reason.Release()

	payload := buffer.NewHeapBuffer(len(encoded))
	_, _ = payload.WriteBytes(encoded)
	scanner := NewFrameScanner(payload)

	frame, ok, err := scanner.Next()
	if err != nil || !ok {
		t.Fatalf("padding ok=%v err=%v", ok, err)
	}
	if frame.(PaddingFrame).Length != 2 {
		t.Fatalf("padding=%+v", frame)
	}
	frame, ok, err = scanner.Next()
	if err != nil || !ok {
		t.Fatalf("ping ok=%v err=%v", ok, err)
	}
	if _, ok := frame.(PingFrame); !ok {
		t.Fatalf("frame=%T, want PingFrame", frame)
	}
	frame, _, err = scanner.Next()
	if err != nil {
		t.Fatal(err)
	}
	ack := frame.(ACKFrame)
	if ack.LargestAcked != 10 || ack.ECN == nil || ack.ECN.CE != 3 || len(ack.AdditionalRanges) != 1 {
		t.Fatalf("ack=%+v", ack)
	}
	frame, _, err = scanner.Next()
	if err != nil {
		t.Fatal(err)
	}
	crypto := frame.(CryptoFrame)
	if crypto.Offset != 4 || string(crypto.Data.Bytes()) != "abc" {
		t.Fatalf("crypto=%+v data=%q", crypto, crypto.Data.Bytes())
	}
	frame, _, err = scanner.Next()
	if err != nil {
		t.Fatal(err)
	}
	stream := frame.(StreamFrame)
	if stream.StreamID != 7 || stream.Offset != 9 || !stream.Fin || string(stream.Data.Bytes()) != "xyz" {
		t.Fatalf("stream=%+v data=%q", stream, stream.Data.Bytes())
	}
	frame, _, err = scanner.Next()
	if err != nil {
		t.Fatal(err)
	}
	closeFrame := frame.(ConnectionCloseFrame)
	if closeFrame.ErrorCode != 42 || closeFrame.FrameType != FrameTypeCrypto || string(closeFrame.Reason.Bytes()) != "bye" {
		t.Fatalf("close=%+v reason=%q", closeFrame, closeFrame.Reason.Bytes())
	}
	frame, _, err = scanner.Next()
	if err != nil {
		t.Fatal(err)
	}
	path := frame.(PathChallengeFrame)
	if path.Data != pathData {
		t.Fatalf("path=%v", path.Data)
	}
	if _, ok, err := scanner.Next(); err != nil || ok {
		t.Fatalf("next ok=%v err=%v", ok, err)
	}

	crypto.Release()
	stream.Release()
	closeFrame.Release()
	if payload.RefCnt() != 1 {
		t.Fatalf("payload ref=%d, want only original ref", payload.RefCnt())
	}
	payload.Release()
}

func TestPacketFrameDecoderPreservesAddressAndPacketContext(t *testing.T) {
	streamData := quicTestBuf("x")
	payload, err := EncodeFrames(buffer.NewHeapAllocator(), PingFrame{}, StreamFrame{StreamID: 1, Data: streamData})
	streamData.Release()
	if err != nil {
		t.Fatal(err)
	}
	defer payload.Release()
	packet := Packet{
		Type:               PacketInitial,
		Version:            Version1,
		DestinationID:      MustConnectionID([]byte{1}),
		SourceID:           MustConnectionID([]byte{2}),
		PacketNumberLength: 1,
		PacketNumber:       9,
		Payload:            payload.Retain(),
	}
	collector := &quicCaptureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("frames", NewPacketFrameDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(udp.Addressed{Message: packet, Addr: quicUDPAddr})
	if len(collector.msgs) != 2 {
		t.Fatalf("frames=%d, want 2", len(collector.msgs))
	}
	addressed := collector.msgs[0].(udp.Addressed)
	event := addressed.Message.(FrameEvent)
	if addressed.Addr.String() != quicUDPAddr.String() || event.Packet.PacketNumber != 9 || event.Packet.Space != PacketNumberSpaceInitial {
		t.Fatalf("addressed=%+v event=%+v", addressed, event)
	}
	for _, msg := range collector.msgs {
		msg.(udp.Addressed).Release()
	}
	if payload.RefCnt() != 1 {
		t.Fatalf("payload ref=%d, want caller ref only", payload.RefCnt())
	}
}

func TestDecodeFrameRejectsMalformedInput(t *testing.T) {
	buf := buffer.NewHeapBuffer(2)
	_, _ = buf.WriteBytes([]byte{byte(FrameTypePathResponse), 1})
	defer buf.Release()

	_, _, err := DecodeFrameAt(buf, 0)
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidFrame)
	}
}
