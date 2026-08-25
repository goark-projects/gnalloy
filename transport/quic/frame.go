package quic

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

const (
	FrameTypePadding          uint64 = 0x00
	FrameTypePing             uint64 = 0x01
	FrameTypeACK              uint64 = 0x02
	FrameTypeACKECN           uint64 = 0x03
	FrameTypeCrypto           uint64 = 0x06
	FrameTypeStreamBase       uint64 = 0x08
	FrameTypePathChallenge    uint64 = 0x1a
	FrameTypePathResponse     uint64 = 0x1b
	FrameTypeConnectionClose  uint64 = 0x1c
	FrameTypeApplicationClose uint64 = 0x1d

	maxACKRanges = 64
)

type PaddingFrame struct {
	Length int
}

type PingFrame struct{}

type ACKRange struct {
	Gap    uint64
	Length uint64
}

type ECNCounts struct {
	ECT0 uint64
	ECT1 uint64
	CE   uint64
}

type ACKFrame struct {
	LargestAcked     uint64
	Delay            uint64
	FirstAckRange    uint64
	AdditionalRanges []ACKRange
	ECN              *ECNCounts
}

type CryptoFrame struct {
	Offset uint64
	Data   buffer.ByteBuf
}

func (f CryptoFrame) Release() {
	if f.Data != nil {
		f.Data.Release()
	}
}

type StreamFrame struct {
	StreamID uint64
	Offset   uint64
	Fin      bool
	Data     buffer.ByteBuf
}

func (f StreamFrame) Release() {
	if f.Data != nil {
		f.Data.Release()
	}
}

type ConnectionCloseFrame struct {
	Application bool
	ErrorCode   uint64
	FrameType   uint64
	Reason      buffer.ByteBuf
}

func (f ConnectionCloseFrame) Release() {
	if f.Reason != nil {
		f.Reason.Release()
	}
}

type PathChallengeFrame struct {
	Data [8]byte
}

type PathResponseFrame struct {
	Data [8]byte
}

// PacketContext 是 frame 分发后保留的 QUIC packet 元信息。
type PacketContext struct {
	Type          PacketType
	Space         PacketNumberSpace
	Version       Version
	DestinationID ConnectionID
	SourceID      ConnectionID
	PacketNumber  uint64
}

type FrameEvent struct {
	Packet        PacketContext
	Frame         any
	Conn          *Connection
	Remote        udp.Address
	NewConnection bool
}

func (e FrameEvent) Release() {
	releaseFrame(e.Frame)
}

func (p Packet) Context() PacketContext {
	return PacketContext{
		Type:          p.Type,
		Space:         PacketSpace(p.Type),
		Version:       p.Version,
		DestinationID: p.DestinationID,
		SourceID:      p.SourceID,
		PacketNumber:  p.PacketNumber,
	}
}

// FrameScanner 在单个 QUIC packet payload 内顺序扫描 frame。
type FrameScanner struct {
	payload buffer.ByteBuf
	offset  int
}

func NewFrameScanner(payload buffer.ByteBuf) FrameScanner {
	return FrameScanner{payload: payload}
}

func (s *FrameScanner) Next() (any, bool, error) {
	if s.payload == nil || s.offset >= s.payload.ReadableBytes() {
		return nil, false, nil
	}
	frame, n, err := DecodeFrameAt(s.payload, s.offset)
	if err != nil {
		return nil, false, err
	}
	s.offset += n
	return frame, true, nil
}

func DecodeFrameAt(payload buffer.ByteBuf, offset int) (any, int, error) {
	if payload == nil || offset < 0 || offset >= payload.ReadableBytes() {
		return nil, 0, ErrInvalidFrame
	}
	data := payload.Bytes()
	frameType, n, err := ParseVarInt(data[offset:])
	if err != nil {
		return nil, 0, err
	}
	if frameType == FrameTypePadding {
		count := 1
		for offset+count < len(data) && data[offset+count] == 0 {
			count++
		}
		return PaddingFrame{Length: count}, count, nil
	}
	idx := offset + n
	switch {
	case frameType == FrameTypePing:
		return PingFrame{}, idx - offset, nil
	case frameType == FrameTypeACK || frameType == FrameTypeACKECN:
		frame, consumed, err := decodeACKFrame(data, idx, frameType == FrameTypeACKECN)
		return frame, consumed + n, err
	case frameType == FrameTypeCrypto:
		frame, consumed, err := decodeCryptoFrame(payload, data, idx)
		return frame, consumed + n, err
	case frameType >= FrameTypeStreamBase && frameType <= FrameTypeStreamBase|0x07:
		frame, consumed, err := decodeStreamFrame(payload, data, idx, frameType)
		return frame, consumed + n, err
	case frameType == FrameTypeConnectionClose || frameType == FrameTypeApplicationClose:
		frame, consumed, err := decodeCloseFrame(payload, data, idx, frameType == FrameTypeApplicationClose)
		return frame, consumed + n, err
	case frameType == FrameTypePathChallenge:
		return decodePathChallengeFrame(data, idx, n)
	case frameType == FrameTypePathResponse:
		return decodePathResponseFrame(data, idx, n)
	default:
		return nil, 0, ErrInvalidFrame
	}
}

func EncodeFrames(alloc buffer.Allocator, frames ...any) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, ErrInvalidFrame
	}
	var encoded []byte
	var err error
	for _, frame := range frames {
		encoded, err = AppendFrame(encoded, frame)
		if err != nil {
			return nil, err
		}
	}
	out, err := alloc.Acquire(len(encoded))
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(encoded); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func AppendFrame(dst []byte, frame any) ([]byte, error) {
	switch f := frame.(type) {
	case PaddingFrame:
		if f.Length < 0 {
			return nil, ErrInvalidFrame
		}
		for i := 0; i < f.Length; i++ {
			dst = append(dst, 0)
		}
		return dst, nil
	case PingFrame:
		return AppendVarInt(dst, FrameTypePing)
	case ACKFrame:
		return appendACKFrame(dst, f)
	case CryptoFrame:
		return appendCryptoFrame(dst, f)
	case StreamFrame:
		return appendStreamFrame(dst, f)
	case ConnectionCloseFrame:
		return appendCloseFrame(dst, f)
	case PathChallengeFrame:
		dst, _ = AppendVarInt(dst, FrameTypePathChallenge)
		return append(dst, f.Data[:]...), nil
	case PathResponseFrame:
		dst, _ = AppendVarInt(dst, FrameTypePathResponse)
		return append(dst, f.Data[:]...), nil
	default:
		return nil, ErrInvalidFrame
	}
}

// PacketFrameDecoder 把 QUIC packet payload 分发成 frame 事件。
type PacketFrameDecoder struct{}

func NewPacketFrameDecoder() *PacketFrameDecoder {
	return &PacketFrameDecoder{}
}

func (d *PacketFrameDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if event, ok := asPacketEvent(msg); ok {
		d.decodePacketEvent(ctx, event)
		return
	}
	if addressed, ok := asUDPAddressed(msg); ok {
		d.decodeAddressed(ctx, addressed)
		return
	}
	packet, ok := asPacket(msg)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	d.decodePacket(ctx, packet, nil)
}

func (d *PacketFrameDecoder) decodeAddressed(ctx *channel.HandlerContext, addressed udp.Addressed) {
	packet, ok := asPacket(addressed.Message)
	if !ok {
		ctx.FireChannelRead(addressed)
		return
	}
	d.decodePacket(ctx, packet, &addressed.Addr)
}

func (d *PacketFrameDecoder) decodePacket(ctx *channel.HandlerContext, packet Packet, addr *udp.Address) {
	d.decodePacketWithMeta(ctx, packet, addr, nil, false)
}

func (d *PacketFrameDecoder) decodePacketEvent(ctx *channel.HandlerContext, event PacketEvent) {
	d.decodePacketWithMeta(ctx, event.Packet, &event.Remote, event.Conn, event.NewConnection)
}

func (d *PacketFrameDecoder) decodePacketWithMeta(ctx *channel.HandlerContext, packet Packet, addr *udp.Address, conn *Connection, newConnection bool) {
	if packet.Payload == nil || packet.Payload.ReadableBytes() == 0 {
		packet.Release()
		return
	}
	meta := packet.Context()
	scanner := NewFrameScanner(packet.Payload)
	for {
		frame, ok, err := scanner.Next()
		if err != nil {
			packet.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		if !ok {
			break
		}
		event := FrameEvent{Packet: meta, Frame: frame, Conn: conn, NewConnection: newConnection}
		if addr != nil {
			event.Remote = *addr
			ctx.FireChannelRead(udp.Addressed{Message: event, Addr: *addr})
		} else {
			ctx.FireChannelRead(event)
		}
	}
	packet.Release()
}

func decodeACKFrame(data []byte, idx int, withECN bool) (ACKFrame, int, error) {
	start := idx
	largest, n, err := readVarIntAt(data, idx)
	if err != nil {
		return ACKFrame{}, 0, err
	}
	idx += n
	delay, n, err := readVarIntAt(data, idx)
	if err != nil {
		return ACKFrame{}, 0, err
	}
	idx += n
	rangeCount, n, err := readVarIntAt(data, idx)
	if err != nil {
		return ACKFrame{}, 0, err
	}
	if rangeCount > maxACKRanges {
		return ACKFrame{}, 0, ErrInvalidFrame
	}
	idx += n
	first, n, err := readVarIntAt(data, idx)
	if err != nil {
		return ACKFrame{}, 0, err
	}
	idx += n
	frame := ACKFrame{LargestAcked: largest, Delay: delay, FirstAckRange: first}
	if rangeCount > 0 {
		frame.AdditionalRanges = make([]ACKRange, int(rangeCount))
	}
	for i := 0; i < int(rangeCount); i++ {
		gap, n, err := readVarIntAt(data, idx)
		if err != nil {
			return ACKFrame{}, 0, err
		}
		idx += n
		length, n, err := readVarIntAt(data, idx)
		if err != nil {
			return ACKFrame{}, 0, err
		}
		idx += n
		frame.AdditionalRanges[i] = ACKRange{Gap: gap, Length: length}
	}
	if withECN {
		ect0, n, err := readVarIntAt(data, idx)
		if err != nil {
			return ACKFrame{}, 0, err
		}
		idx += n
		ect1, n, err := readVarIntAt(data, idx)
		if err != nil {
			return ACKFrame{}, 0, err
		}
		idx += n
		ce, n, err := readVarIntAt(data, idx)
		if err != nil {
			return ACKFrame{}, 0, err
		}
		idx += n
		frame.ECN = &ECNCounts{ECT0: ect0, ECT1: ect1, CE: ce}
	}
	return frame, idx - start, nil
}

func decodeCryptoFrame(payload buffer.ByteBuf, data []byte, idx int) (CryptoFrame, int, error) {
	start := idx
	offset, n, err := readVarIntAt(data, idx)
	if err != nil {
		return CryptoFrame{}, 0, err
	}
	idx += n
	length, n, err := readVarIntAt(data, idx)
	if err != nil {
		return CryptoFrame{}, 0, err
	}
	idx += n
	if length > uint64(len(data)-idx) {
		return CryptoFrame{}, 0, ErrInvalidFrame
	}
	frame := CryptoFrame{Offset: offset}
	if length > 0 {
		slice, err := payload.Slice(payload.ReaderIndex()+idx, int(length))
		if err != nil {
			return CryptoFrame{}, 0, err
		}
		frame.Data = slice
	}
	return frame, idx - start + int(length), nil
}

func decodeStreamFrame(payload buffer.ByteBuf, data []byte, idx int, frameType uint64) (StreamFrame, int, error) {
	start := idx
	streamID, n, err := readVarIntAt(data, idx)
	if err != nil {
		return StreamFrame{}, 0, err
	}
	idx += n
	frame := StreamFrame{StreamID: streamID, Fin: frameType&0x01 != 0}
	if frameType&0x04 != 0 {
		frame.Offset, n, err = readVarIntAt(data, idx)
		if err != nil {
			return StreamFrame{}, 0, err
		}
		idx += n
	}
	length := uint64(len(data) - idx)
	if frameType&0x02 != 0 {
		length, n, err = readVarIntAt(data, idx)
		if err != nil {
			return StreamFrame{}, 0, err
		}
		idx += n
		if length > uint64(len(data)-idx) {
			return StreamFrame{}, 0, ErrInvalidFrame
		}
	}
	if length > 0 {
		slice, err := payload.Slice(payload.ReaderIndex()+idx, int(length))
		if err != nil {
			return StreamFrame{}, 0, err
		}
		frame.Data = slice
	}
	return frame, idx - start + int(length), nil
}

func decodeCloseFrame(payload buffer.ByteBuf, data []byte, idx int, application bool) (ConnectionCloseFrame, int, error) {
	start := idx
	errorCode, n, err := readVarIntAt(data, idx)
	if err != nil {
		return ConnectionCloseFrame{}, 0, err
	}
	idx += n
	frameType := uint64(0)
	if !application {
		frameType, n, err = readVarIntAt(data, idx)
		if err != nil {
			return ConnectionCloseFrame{}, 0, err
		}
		idx += n
	}
	reasonLen, n, err := readVarIntAt(data, idx)
	if err != nil {
		return ConnectionCloseFrame{}, 0, err
	}
	idx += n
	if reasonLen > uint64(len(data)-idx) {
		return ConnectionCloseFrame{}, 0, ErrInvalidFrame
	}
	frame := ConnectionCloseFrame{Application: application, ErrorCode: errorCode, FrameType: frameType}
	if reasonLen > 0 {
		slice, err := payload.Slice(payload.ReaderIndex()+idx, int(reasonLen))
		if err != nil {
			return ConnectionCloseFrame{}, 0, err
		}
		frame.Reason = slice
	}
	return frame, idx - start + int(reasonLen), nil
}

func decodePathChallengeFrame(data []byte, idx int, typeLen int) (PathChallengeFrame, int, error) {
	if len(data)-idx < 8 {
		return PathChallengeFrame{}, 0, ErrInvalidFrame
	}
	var frame PathChallengeFrame
	copy(frame.Data[:], data[idx:idx+8])
	return frame, typeLen + 8, nil
}

func decodePathResponseFrame(data []byte, idx int, typeLen int) (PathResponseFrame, int, error) {
	if len(data)-idx < 8 {
		return PathResponseFrame{}, 0, ErrInvalidFrame
	}
	var frame PathResponseFrame
	copy(frame.Data[:], data[idx:idx+8])
	return frame, typeLen + 8, nil
}

func appendACKFrame(dst []byte, f ACKFrame) ([]byte, error) {
	frameType := FrameTypeACK
	if f.ECN != nil {
		frameType = FrameTypeACKECN
	}
	var err error
	if dst, err = AppendVarInt(dst, frameType); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, f.LargestAcked); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, f.Delay); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, uint64(len(f.AdditionalRanges))); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, f.FirstAckRange); err != nil {
		return nil, err
	}
	for _, r := range f.AdditionalRanges {
		if dst, err = AppendVarInt(dst, r.Gap); err != nil {
			return nil, err
		}
		if dst, err = AppendVarInt(dst, r.Length); err != nil {
			return nil, err
		}
	}
	if f.ECN != nil {
		if dst, err = AppendVarInt(dst, f.ECN.ECT0); err != nil {
			return nil, err
		}
		if dst, err = AppendVarInt(dst, f.ECN.ECT1); err != nil {
			return nil, err
		}
		dst, err = AppendVarInt(dst, f.ECN.CE)
	}
	return dst, err
}

func appendCryptoFrame(dst []byte, f CryptoFrame) ([]byte, error) {
	if f.Data == nil {
		return nil, ErrInvalidFrame
	}
	var err error
	if dst, err = AppendVarInt(dst, FrameTypeCrypto); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, f.Offset); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, uint64(f.Data.ReadableBytes())); err != nil {
		return nil, err
	}
	return append(dst, f.Data.Bytes()...), nil
}

func appendStreamFrame(dst []byte, f StreamFrame) ([]byte, error) {
	if f.Data == nil {
		return nil, ErrInvalidFrame
	}
	frameType := FrameTypeStreamBase | 0x02
	if f.Offset > 0 {
		frameType |= 0x04
	}
	if f.Fin {
		frameType |= 0x01
	}
	var err error
	if dst, err = AppendVarInt(dst, frameType); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, f.StreamID); err != nil {
		return nil, err
	}
	if f.Offset > 0 {
		if dst, err = AppendVarInt(dst, f.Offset); err != nil {
			return nil, err
		}
	}
	if dst, err = AppendVarInt(dst, uint64(f.Data.ReadableBytes())); err != nil {
		return nil, err
	}
	return append(dst, f.Data.Bytes()...), nil
}

func appendCloseFrame(dst []byte, f ConnectionCloseFrame) ([]byte, error) {
	frameType := FrameTypeConnectionClose
	if f.Application {
		frameType = FrameTypeApplicationClose
	}
	var err error
	if dst, err = AppendVarInt(dst, frameType); err != nil {
		return nil, err
	}
	if dst, err = AppendVarInt(dst, f.ErrorCode); err != nil {
		return nil, err
	}
	if !f.Application {
		if dst, err = AppendVarInt(dst, f.FrameType); err != nil {
			return nil, err
		}
	}
	reasonLen := 0
	if f.Reason != nil {
		reasonLen = f.Reason.ReadableBytes()
	}
	if dst, err = AppendVarInt(dst, uint64(reasonLen)); err != nil {
		return nil, err
	}
	if reasonLen > 0 {
		dst = append(dst, f.Reason.Bytes()...)
	}
	return dst, nil
}

func readVarIntAt(data []byte, idx int) (uint64, int, error) {
	if idx < 0 || idx >= len(data) {
		return 0, 0, ErrInvalidFrame
	}
	return ParseVarInt(data[idx:])
}

func asPacket(msg any) (Packet, bool) {
	switch v := msg.(type) {
	case Packet:
		return v, true
	case *Packet:
		if v == nil {
			return Packet{}, false
		}
		return *v, true
	default:
		return Packet{}, false
	}
}

func asPacketEvent(msg any) (PacketEvent, bool) {
	switch v := msg.(type) {
	case PacketEvent:
		return v, true
	case *PacketEvent:
		if v == nil {
			return PacketEvent{}, false
		}
		return *v, true
	default:
		return PacketEvent{}, false
	}
}

func releaseFrame(frame any) {
	if releasable, ok := frame.(interface{ Release() }); ok {
		releasable.Release()
	}
}
