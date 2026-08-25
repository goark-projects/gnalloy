package http2

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	ClientPreface       = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	FrameHeaderSize     = 9
	DefaultMaxFrameSize = 16 * 1024
	MaxFrameSizeLimit   = 16*1024*1024 - 1
)

type FrameType uint8

const (
	FrameData FrameType = iota
	FrameHeaders
	FramePriority
	FrameRSTStream
	FrameSettings
	FramePushPromise
	FramePing
	FrameGoAway
	FrameWindowUpdate
	FrameContinuation
)

type Flags uint8

const (
	FlagEndStream  Flags = 0x1
	FlagAck        Flags = 0x1
	FlagEndHeaders Flags = 0x4
	FlagPadded     Flags = 0x8
	FlagPriority   Flags = 0x20
)

type FrameHeader struct {
	Length   int
	Type     FrameType
	Flags    Flags
	StreamID StreamID
}

type Frame struct {
	Type     FrameType
	Flags    Flags
	StreamID StreamID
	Payload  buffer.ByteBuf
}

func (f Frame) Header() FrameHeader {
	length := 0
	if f.Payload != nil {
		length = f.Payload.ReadableBytes()
	}
	return FrameHeader{Length: length, Type: f.Type, Flags: f.Flags, StreamID: f.StreamID}
}

func (f Frame) Release() {
	if f.Payload != nil {
		f.Payload.Release()
	}
}

func SettingsAck() Frame {
	return Frame{Type: FrameSettings, Flags: FlagAck}
}

func AppendFrameHeader(dst []byte, h FrameHeader) ([]byte, error) {
	if h.Length < 0 || h.Length > MaxFrameSizeLimit {
		return nil, ErrInvalidFrame
	}
	if h.StreamID > maxStreamID {
		return nil, ErrInvalidStreamID
	}
	dst = append(dst,
		byte(h.Length>>16),
		byte(h.Length>>8),
		byte(h.Length),
		byte(h.Type),
		byte(h.Flags),
		byte(uint32(h.StreamID)>>24),
		byte(uint32(h.StreamID)>>16),
		byte(uint32(h.StreamID)>>8),
		byte(h.StreamID),
	)
	dst[len(dst)-4] &= 0x7f
	return dst, nil
}

func ParseFrameHeader(src []byte) (FrameHeader, error) {
	if len(src) < FrameHeaderSize {
		return FrameHeader{}, ErrInvalidFrame
	}
	length := int(src[0])<<16 | int(src[1])<<8 | int(src[2])
	streamID := StreamID(uint32(src[5]&0x7f)<<24 | uint32(src[6])<<16 | uint32(src[7])<<8 | uint32(src[8]))
	return FrameHeader{
		Length:   length,
		Type:     FrameType(src[3]),
		Flags:    Flags(src[4]),
		StreamID: streamID,
	}, nil
}

type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameSize int
}

func NewFrameDecoder(maxFrameSize int) (*FrameDecoder, error) {
	if maxFrameSize <= 0 {
		maxFrameSize = DefaultMaxFrameSize
	}
	if maxFrameSize > MaxFrameSizeLimit {
		return nil, ErrFrameTooLarge
	}
	d := &FrameDecoder{maxFrameSize: maxFrameSize}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *FrameDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < FrameHeaderSize {
		return nil, nil
	}
	headerBytes, err := in.Slice(in.ReaderIndex(), FrameHeaderSize)
	if err != nil {
		return nil, err
	}
	header, err := ParseFrameHeader(headerBytes.Bytes())
	headerBytes.Release()
	if err != nil {
		return nil, err
	}
	if header.Length > d.maxFrameSize {
		return nil, ErrFrameTooLarge
	}
	total := FrameHeaderSize + header.Length
	if in.ReadableBytes() < total {
		return nil, nil
	}
	if err := in.SkipBytes(FrameHeaderSize); err != nil {
		return nil, err
	}
	var payload buffer.ByteBuf
	if header.Length > 0 {
		payload, err = in.Slice(in.ReaderIndex(), header.Length)
		if err != nil {
			return nil, err
		}
		if err := in.SkipBytes(header.Length); err != nil {
			payload.Release()
			return nil, err
		}
	}
	return Frame{Type: header.Type, Flags: header.Flags, StreamID: header.StreamID, Payload: payload}, nil
}

type FrameEncoder struct{}

func NewFrameEncoder() *FrameEncoder {
	return &FrameEncoder{}
}

func (e *FrameEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(Frame)
	if !ok {
		return ctx.Write(msg)
	}
	header, err := AppendFrameHeader(nil, frame.Header())
	if err != nil {
		frame.Release()
		return err
	}
	out, err := ctx.Channel().Allocator().Acquire(FrameHeaderSize)
	if err != nil {
		frame.Release()
		return err
	}
	if _, err := out.WriteBytes(header); err != nil {
		out.Release()
		frame.Release()
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		frame.Release()
		return err
	}
	if frame.Payload == nil {
		return nil
	}
	return codec.WriteOutboundBuffer(ctx, frame.Payload)
}
