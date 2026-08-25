package mqtt

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const maxRemainingLength = 268435455

const (
	PacketConnect     byte = 1
	PacketConnAck     byte = 2
	PacketPublish     byte = 3
	PacketPubAck      byte = 4
	PacketPubRec      byte = 5
	PacketPubRel      byte = 6
	PacketPubComp     byte = 7
	PacketSubscribe   byte = 8
	PacketSubAck      byte = 9
	PacketUnsubscribe byte = 10
	PacketUnsubAck    byte = 11
	PacketPingReq     byte = 12
	PacketPingResp    byte = 13
	PacketDisconnect  byte = 14
	PacketAuth        byte = 15
)

type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameLength int
}

func NewFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	if maxFrameLength <= 0 || maxFrameLength > maxRemainingLength+5 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &FrameDecoder{maxFrameLength: maxFrameLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *FrameDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 2 {
		return nil, nil
	}
	remaining, header, ok, err := readRemainingLength(in)
	if err != nil || !ok {
		return nil, err
	}
	total := header + remaining
	if total > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
	}
	if in.ReadableBytes() < total {
		return nil, nil
	}
	frame, err := in.Slice(in.ReaderIndex(), total)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(total); err != nil {
		frame.Release()
		return nil, err
	}
	return frame, nil
}

type FramePrepender struct{}

func NewFramePrepender() *FramePrepender {
	return &FramePrepender{}
}

func (p *FramePrepender) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(Frame)
	if !ok {
		return ctx.Write(msg)
	}
	if frame.TypeFlags == 0 || frame.RemainingLength() > maxRemainingLength {
		return codec.ErrInvalidFrameLength
	}
	headerLen := 1 + encodedRemainingLengthSize(frame.RemainingLength())
	header, err := ctx.Channel().Allocator().Acquire(headerLen)
	if err != nil {
		frame.Release()
		return err
	}
	var tmp [5]byte
	tmp[0] = frame.TypeFlags
	n := putRemainingLength(tmp[1:], frame.RemainingLength())
	if _, err := header.WriteBytes(tmp[:1+n]); err != nil {
		header.Release()
		frame.Release()
		return err
	}
	if err := ctx.Write(header); err != nil {
		header.Release()
		frame.Release()
		return err
	}
	if frame.Payload == nil {
		return nil
	}
	return codec.WriteOutboundBuffer(ctx, frame.Payload)
}

type Frame struct {
	TypeFlags byte
	Payload   buffer.ByteBuf
}

func NewFrame(packetType byte, flags byte, payload buffer.ByteBuf) Frame {
	return Frame{TypeFlags: packetType<<4 | (flags & 0x0f), Payload: payload}
}

func PingResp() Frame {
	return NewFrame(PacketPingResp, 0, nil)
}

func (f Frame) PacketType() byte {
	return f.TypeFlags >> 4
}

func (f Frame) Flags() byte {
	return f.TypeFlags & 0x0f
}

func (f Frame) RemainingLength() int {
	if f.Payload == nil {
		return 0
	}
	return f.Payload.ReadableBytes()
}

func (f Frame) Release() {
	if f.Payload != nil {
		f.Payload.Release()
	}
}

type TypedFrameDecoder struct {
	*codec.MessageToMessageDecoder
}

func NewTypedFrameDecoder() *TypedFrameDecoder {
	d := &TypedFrameDecoder{}
	d.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(d)
	return d
}

func (d *TypedFrameDecoder) AcceptInboundMessage(msg any) bool {
	_, ok := msg.(buffer.ByteBuf)
	return ok
}

func (d *TypedFrameDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	buf := msg.(buffer.ByteBuf)
	if buf.ReadableBytes() < 2 {
		return codec.ErrInvalidFrameLength
	}
	typeFlags, _ := buf.GetByte(buf.ReaderIndex())
	remaining, header, ok, err := readRemainingLengthFrom(buf, buf.ReaderIndex())
	if err != nil {
		return err
	}
	if !ok || buf.ReaderIndex()+header+remaining != buf.WriterIndex() {
		return codec.ErrInvalidFrameLength
	}
	var payload buffer.ByteBuf
	if remaining > 0 {
		payload, err = buf.Slice(buf.ReaderIndex()+header, remaining)
		if err != nil {
			return err
		}
	}
	out.Add(Frame{TypeFlags: typeFlags, Payload: payload})
	return nil
}

func readRemainingLength(in *buffer.CompositeByteBuf) (int, int, bool, error) {
	multiplier := 1
	value := 0
	for i := 1; i <= 4; i++ {
		if in.ReadableBytes() <= i {
			return 0, 0, false, nil
		}
		encoded, ok := in.GetByte(in.ReaderIndex() + i)
		if !ok {
			return 0, 0, false, nil
		}
		value += int(encoded&127) * multiplier
		if encoded&128 == 0 {
			return value, i + 1, true, nil
		}
		multiplier *= 128
	}
	return 0, 0, false, codec.ErrInvalidLengthField
}

func readRemainingLengthFrom(in buffer.ByteBuf, index int) (int, int, bool, error) {
	multiplier := 1
	value := 0
	for i := 1; i <= 4; i++ {
		if in.WriterIndex()-index <= i {
			return 0, 0, false, nil
		}
		encoded, ok := in.GetByte(index + i)
		if !ok {
			return 0, 0, false, nil
		}
		value += int(encoded&127) * multiplier
		if encoded&128 == 0 {
			return value, i + 1, true, nil
		}
		multiplier *= 128
	}
	return 0, 0, false, codec.ErrInvalidLengthField
}

func encodedRemainingLengthSize(v int) int {
	size := 1
	for v >= 128 {
		v /= 128
		size++
	}
	return size
}

func putRemainingLength(dst []byte, v int) int {
	i := 0
	for {
		encoded := byte(v % 128)
		v /= 128
		if v > 0 {
			encoded |= 128
		}
		dst[i] = encoded
		i++
		if v == 0 {
			return i
		}
	}
}
