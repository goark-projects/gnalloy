package http3

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type Decoder struct {
	*codec.ByteToMessageDecoder
	maxFrameSize int
	maxSettings  int
}

func NewDecoder(maxFrameSize int) (*Decoder, error) {
	if maxFrameSize <= 0 {
		maxFrameSize = DefaultMaxFrameSize
	}
	d := &Decoder{maxFrameSize: maxFrameSize, maxSettings: defaultSettings}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *Decoder) SetMaxSettings(maxSettings int) error {
	if maxSettings <= 0 {
		return ErrTooManySettings
	}
	d.maxSettings = maxSettings
	return nil
}

func (d *Decoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	reader := in.ReaderIndex()
	frameType, typeLen, ok, err := readVarInt(in, reader)
	if err != nil || !ok {
		return nil, err
	}
	length, lengthLen, ok, err := readVarInt(in, reader+typeLen)
	if err != nil || !ok {
		return nil, err
	}
	if length > uint64(d.maxFrameSize) {
		return nil, ErrFrameTooLarge
	}
	payload := reader + typeLen + lengthLen
	total := typeLen + lengthLen + int(length)
	if in.ReadableBytes() < total {
		return nil, nil
	}
	msg, err := d.decodeFrame(in, FrameType(frameType), payload, int(length))
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(total); err != nil {
		releaseMessage(msg)
		return nil, err
	}
	return msg, nil
}

func (d *Decoder) decodeFrame(in *buffer.CompositeByteBuf, frameType FrameType, payload int, length int) (any, error) {
	switch frameType {
	case FrameData:
		data, err := slicePayload(in, payload, length)
		if err != nil {
			return nil, err
		}
		return DataFrame{Data: data}, nil
	case FrameHeaders:
		headerBlock, err := slicePayload(in, payload, length)
		if err != nil {
			return nil, err
		}
		return HeadersFrame{HeaderBlock: headerBlock}, nil
	case FrameCancelPush:
		pushID, err := parseSingleVarInt(in, payload, length)
		if err != nil {
			return nil, err
		}
		return CancelPushFrame{PushID: pushID}, nil
	case FrameSettings:
		return d.decodeSettings(in, payload, length)
	case FramePushPromise:
		pushID, n, ok, err := readVarInt(in, payload)
		if err != nil || !ok || n > length {
			return nil, ErrInvalidFrame
		}
		headerBlock, err := slicePayload(in, payload+n, length-n)
		if err != nil {
			return nil, err
		}
		return PushPromiseFrame{PushID: pushID, HeaderBlock: headerBlock}, nil
	case FrameGoAway:
		id, err := parseSingleVarInt(in, payload, length)
		if err != nil {
			return nil, err
		}
		return GoAwayFrame{ID: id}, nil
	case FrameMaxPushID:
		pushID, err := parseSingleVarInt(in, payload, length)
		if err != nil {
			return nil, err
		}
		return MaxPushIDFrame{PushID: pushID}, nil
	case FramePriorityUpdateStream, FramePriorityUpdatePush:
		elementID, n, ok, err := readVarInt(in, payload)
		if err != nil || !ok || n > length {
			return nil, ErrInvalidFrame
		}
		fieldValue, err := slicePayload(in, payload+n, length-n)
		if err != nil {
			return nil, err
		}
		return PriorityUpdateFrame{Type: frameType, ElementID: elementID, FieldValue: fieldValue}, nil
	default:
		data, err := slicePayload(in, payload, length)
		if err != nil {
			return nil, err
		}
		return UnknownFrame{Type: frameType, Payload: data}, nil
	}
}

func (d *Decoder) decodeSettings(in *buffer.CompositeByteBuf, payload int, length int) (SettingsFrame, error) {
	end := payload + length
	idx := payload
	var settings []Setting
	seen := make(map[uint64]struct{}, min(d.maxSettings, 8))
	for idx < end {
		id, n, ok, err := readVarInt(in, idx)
		if err != nil || !ok {
			return SettingsFrame{}, ErrInvalidFrame
		}
		idx += n
		value, n, ok, err := readVarInt(in, idx)
		if err != nil || !ok {
			return SettingsFrame{}, ErrInvalidFrame
		}
		idx += n
		if _, exists := seen[id]; exists {
			return SettingsFrame{}, ErrDuplicateSetting
		}
		seen[id] = struct{}{}
		if len(settings) >= d.maxSettings {
			return SettingsFrame{}, ErrTooManySettings
		}
		settings = append(settings, Setting{ID: id, Value: value})
	}
	if idx != end {
		return SettingsFrame{}, ErrInvalidFrame
	}
	return SettingsFrame{Settings: settings}, nil
}

type Encoder struct{}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	switch frame := msg.(type) {
	case DataFrame:
		return writePayloadFrame(ctx, FrameData, frame.Data)
	case *DataFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return writePayloadFrame(ctx, FrameData, frame.Data)
	case HeadersFrame:
		return writePayloadFrame(ctx, FrameHeaders, frame.HeaderBlock)
	case *HeadersFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return writePayloadFrame(ctx, FrameHeaders, frame.HeaderBlock)
	case CancelPushFrame:
		return writeVarIntFrame(ctx, FrameCancelPush, frame.PushID)
	case SettingsFrame:
		return writeSettingsFrame(ctx, frame)
	case PushPromiseFrame:
		return writeVarIntAndPayloadFrame(ctx, FramePushPromise, frame.PushID, frame.HeaderBlock)
	case *PushPromiseFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return writeVarIntAndPayloadFrame(ctx, FramePushPromise, frame.PushID, frame.HeaderBlock)
	case GoAwayFrame:
		return writeVarIntFrame(ctx, FrameGoAway, frame.ID)
	case MaxPushIDFrame:
		return writeVarIntFrame(ctx, FrameMaxPushID, frame.PushID)
	case PriorityUpdateFrame:
		if frame.Type != FramePriorityUpdateStream && frame.Type != FramePriorityUpdatePush {
			frame.Release()
			return ErrInvalidFrame
		}
		return writeVarIntAndPayloadFrame(ctx, frame.Type, frame.ElementID, frame.FieldValue)
	case *PriorityUpdateFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		if frame.Type != FramePriorityUpdateStream && frame.Type != FramePriorityUpdatePush {
			frame.Release()
			return ErrInvalidFrame
		}
		return writeVarIntAndPayloadFrame(ctx, frame.Type, frame.ElementID, frame.FieldValue)
	case UnknownFrame:
		return writePayloadFrame(ctx, frame.Type, frame.Payload)
	case *UnknownFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return writePayloadFrame(ctx, frame.Type, frame.Payload)
	default:
		return ctx.Write(msg)
	}
}

func writePayloadFrame(ctx *channel.HandlerContext, frameType FrameType, payload buffer.ByteBuf) error {
	header, err := appendFrameHeader(nil, frameType, uint64(readable(payload)))
	if err != nil {
		if payload != nil {
			payload.Release()
		}
		return err
	}
	if err := writeBytes(ctx, header); err != nil {
		if payload != nil {
			payload.Release()
		}
		return err
	}
	if payload == nil || payload.ReadableBytes() == 0 {
		if payload != nil {
			payload.Release()
		}
		return nil
	}
	if err := ctx.Write(payload); err != nil {
		payload.Release()
		return err
	}
	return nil
}

func writeVarIntFrame(ctx *channel.HandlerContext, frameType FrameType, value uint64) error {
	var payload []byte
	payload, err := appendVarInt(payload, value)
	if err != nil {
		return err
	}
	header, err := appendFrameHeader(nil, frameType, uint64(len(payload)))
	if err != nil {
		return err
	}
	header = append(header, payload...)
	return writeBytes(ctx, header)
}

func writeSettingsFrame(ctx *channel.HandlerContext, frame SettingsFrame) error {
	var payload []byte
	seen := make(map[uint64]struct{}, min(len(frame.Settings), 8))
	for _, setting := range frame.Settings {
		if _, exists := seen[setting.ID]; exists {
			return ErrDuplicateSetting
		}
		seen[setting.ID] = struct{}{}
		var err error
		payload, err = appendVarInt(payload, setting.ID)
		if err != nil {
			return err
		}
		payload, err = appendVarInt(payload, setting.Value)
		if err != nil {
			return err
		}
	}
	header, err := appendFrameHeader(nil, FrameSettings, uint64(len(payload)))
	if err != nil {
		return err
	}
	header = append(header, payload...)
	return writeBytes(ctx, header)
}

func writeVarIntAndPayloadFrame(ctx *channel.HandlerContext, frameType FrameType, value uint64, payload buffer.ByteBuf) error {
	var prefix []byte
	prefix, err := appendVarInt(prefix, value)
	if err != nil {
		if payload != nil {
			payload.Release()
		}
		return err
	}
	header, err := appendFrameHeader(nil, frameType, uint64(len(prefix)+readable(payload)))
	if err != nil {
		if payload != nil {
			payload.Release()
		}
		return err
	}
	header = append(header, prefix...)
	if err := writeBytes(ctx, header); err != nil {
		if payload != nil {
			payload.Release()
		}
		return err
	}
	if payload == nil || payload.ReadableBytes() == 0 {
		if payload != nil {
			payload.Release()
		}
		return nil
	}
	if err := ctx.Write(payload); err != nil {
		payload.Release()
		return err
	}
	return nil
}

func appendFrameHeader(dst []byte, frameType FrameType, length uint64) ([]byte, error) {
	var err error
	if dst, err = appendVarInt(dst, uint64(frameType)); err != nil {
		return nil, err
	}
	return appendVarInt(dst, length)
}

func parseSingleVarInt(in *buffer.CompositeByteBuf, payload int, length int) (uint64, error) {
	value, n, ok, err := readVarInt(in, payload)
	if err != nil || !ok || n != length {
		return 0, ErrInvalidFrame
	}
	return value, nil
}

func slicePayload(in *buffer.CompositeByteBuf, index int, length int) (buffer.ByteBuf, error) {
	if length == 0 {
		return nil, nil
	}
	return in.Slice(index, length)
}

func readable(buf buffer.ByteBuf) int {
	if buf == nil {
		return 0
	}
	return buf.ReadableBytes()
}

func releaseMessage(msg any) {
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

func writeBytes(ctx *channel.HandlerContext, data []byte) error {
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return err
	}
	return codec.WriteOutboundBuffer(ctx, out)
}

func readVarInt(in *buffer.CompositeByteBuf, index int) (uint64, int, bool, error) {
	if in.WriterIndex()-index < 1 {
		return 0, 0, false, nil
	}
	first, ok := in.GetByte(index)
	if !ok {
		return 0, 0, false, nil
	}
	n := 1 << (first >> 6)
	if in.WriterIndex()-index < int(n) {
		return 0, 0, false, nil
	}
	switch n {
	case 1:
		return uint64(first & 0x3f), 1, true, nil
	case 2:
		second, _ := in.GetByte(index + 1)
		return uint64(first&0x3f)<<8 | uint64(second), 2, true, nil
	case 4:
		value := uint64(first&0x3f) << 24
		for i := 1; i < 4; i++ {
			b, _ := in.GetByte(index + i)
			value |= uint64(b) << uint(8*(3-i))
		}
		return value, 4, true, nil
	case 8:
		value := uint64(first&0x3f) << 56
		for i := 1; i < 8; i++ {
			b, _ := in.GetByte(index + i)
			value |= uint64(b) << uint(8*(7-i))
		}
		return value, 8, true, nil
	default:
		return 0, 0, false, ErrInvalidVarInt
	}
}

func appendVarInt(dst []byte, v uint64) ([]byte, error) {
	switch {
	case v <= 63:
		return append(dst, byte(v)), nil
	case v <= 16383:
		return append(dst, byte(v>>8)|0x40, byte(v)), nil
	case v <= 1073741823:
		return append(dst, byte(v>>24)|0x80, byte(v>>16), byte(v>>8), byte(v)), nil
	case v <= maxVarInt:
		return append(dst, byte(v>>56)|0xc0, byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v)), nil
	default:
		return nil, ErrInvalidVarInt
	}
}
