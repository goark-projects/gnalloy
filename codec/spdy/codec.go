package spdy

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/internal/message"
)

type Decoder struct {
	*codec.ByteToMessageDecoder
	version        uint16
	maxFrameLength int
	maxSettings    int
}

func NewDecoder(version uint16, maxFrameLength int) (*Decoder, error) {
	if version == 0 {
		version = Version3
	}
	if maxFrameLength <= 0 || maxFrameLength > maxPayloadLength+headerSize {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &Decoder{version: version, maxFrameLength: maxFrameLength, maxSettings: defaultSettings}
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
	if in.ReadableBytes() < headerSize {
		return nil, nil
	}
	reader := in.ReaderIndex()
	header, err := decodeFrameHeader(in, reader)
	if err != nil {
		return nil, err
	}
	total := headerSize + header.length
	if total > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
	}
	if in.ReadableBytes() < total {
		return nil, nil
	}
	if header.control {
		if header.version != d.version {
			return nil, ErrUnsupportedVersion
		}
		msg, err := d.decodeControl(in, reader+headerSize, header.kind, header.flags, header.length)
		if err != nil {
			return nil, err
		}
		if err := in.SkipBytes(total); err != nil {
			releaseMessage(msg)
			return nil, err
		}
		return msg, nil
	}
	if header.stream == 0 {
		return nil, ErrInvalidFrame
	}
	data, err := slicePayload(in, reader+headerSize, header.length)
	if err != nil {
		return nil, err
	}
	msg := DataFrame{StreamID: header.stream, Flags: header.flags, Data: data}
	if err := in.SkipBytes(total); err != nil {
		msg.Release()
		return nil, err
	}
	return msg, nil
}

func (d *Decoder) decodeControl(in *buffer.CompositeByteBuf, payload int, frameType FrameType, flags byte, length int) (any, error) {
	switch frameType {
	case FrameTypeSynStream:
		if length < 10 {
			return nil, ErrInvalidFrame
		}
		streamID, err := readStreamID(in, payload)
		if err != nil {
			return nil, err
		}
		associatedID, err := readStreamID(in, payload+4)
		if err != nil {
			return nil, err
		}
		priorityByte, _ := in.GetByte(payload + 8)
		headerBlock, err := slicePayload(in, payload+10, length-10)
		if err != nil {
			return nil, err
		}
		if streamID == 0 {
			headerBlock.Release()
			return nil, ErrInvalidFrame
		}
		return SynStreamFrame{
			StreamID:             streamID,
			AssociatedToStreamID: associatedID,
			Priority:             (priorityByte >> 5) & 0x07,
			Flags:                flags,
			HeaderBlock:          headerBlock,
		}, nil
	case FrameTypeSynReply:
		if length < 4 {
			return nil, ErrInvalidFrame
		}
		streamID, err := readStreamID(in, payload)
		if err != nil {
			return nil, err
		}
		headerBlock, err := slicePayload(in, payload+4, length-4)
		if err != nil {
			return nil, err
		}
		if streamID == 0 {
			headerBlock.Release()
			return nil, ErrInvalidFrame
		}
		return SynReplyFrame{StreamID: streamID, Flags: flags, HeaderBlock: headerBlock}, nil
	case FrameTypeRSTStream:
		if flags != 0 || length != 8 {
			return nil, ErrInvalidFrame
		}
		streamID, err := readStreamID(in, payload)
		if err != nil {
			return nil, err
		}
		status, err := readUint32(in, payload+4)
		if err != nil {
			return nil, err
		}
		if streamID == 0 || status == 0 {
			return nil, ErrInvalidFrame
		}
		return RSTStreamFrame{StreamID: streamID, StatusCode: status}, nil
	case FrameTypeSettings:
		return d.decodeSettings(in, payload, flags, length)
	case FrameTypePing:
		if flags != 0 || length != 4 {
			return nil, ErrInvalidFrame
		}
		id, err := readUint32(in, payload)
		if err != nil {
			return nil, err
		}
		return PingFrame{ID: id}, nil
	case FrameTypeGoAway:
		if flags != 0 || length != 8 {
			return nil, ErrInvalidFrame
		}
		lastGood, err := readStreamID(in, payload)
		if err != nil {
			return nil, err
		}
		status, err := readUint32(in, payload+4)
		if err != nil {
			return nil, err
		}
		return GoAwayFrame{LastGoodStreamID: lastGood, StatusCode: status}, nil
	case FrameTypeHeaders:
		if length < 4 {
			return nil, ErrInvalidFrame
		}
		streamID, err := readStreamID(in, payload)
		if err != nil {
			return nil, err
		}
		headerBlock, err := slicePayload(in, payload+4, length-4)
		if err != nil {
			return nil, err
		}
		if streamID == 0 {
			headerBlock.Release()
			return nil, ErrInvalidFrame
		}
		return HeadersFrame{StreamID: streamID, Flags: flags, HeaderBlock: headerBlock}, nil
	case FrameTypeWindowUpdate:
		if flags != 0 || length != 8 {
			return nil, ErrInvalidFrame
		}
		streamID, err := readStreamID(in, payload)
		if err != nil {
			return nil, err
		}
		delta, err := readStreamID(in, payload+4)
		if err != nil {
			return nil, err
		}
		if delta == 0 {
			return nil, ErrInvalidFrame
		}
		return WindowUpdateFrame{StreamID: streamID, DeltaWindowSize: delta}, nil
	default:
		payloadBuf, err := slicePayload(in, payload, length)
		if err != nil {
			return nil, err
		}
		return UnknownFrame{Version: d.version, Type: frameType, Flags: flags, Payload: payloadBuf}, nil
	}
}

func (d *Decoder) decodeSettings(in *buffer.CompositeByteBuf, payload int, flags byte, length int) (SettingsFrame, error) {
	if length < 4 {
		return SettingsFrame{}, ErrInvalidFrame
	}
	count, err := readStreamID(in, payload)
	if err != nil {
		return SettingsFrame{}, err
	}
	if int(count) > d.maxSettings {
		return SettingsFrame{}, ErrTooManySettings
	}
	if length-4 != int(count)*8 {
		return SettingsFrame{}, ErrInvalidFrame
	}
	settings := make([]Setting, int(count))
	offset := payload + 4
	for i := range settings {
		settingFlags, _ := in.GetByte(offset)
		id, err := readMedium(in, offset+1)
		if err != nil {
			return SettingsFrame{}, err
		}
		value, err := readUint32(in, offset+4)
		if err != nil {
			return SettingsFrame{}, err
		}
		settings[i] = Setting{ID: id, Value: value, Flags: settingFlags}
		offset += 8
	}
	return SettingsFrame{Flags: flags, Settings: settings}, nil
}

type Encoder struct {
	version uint16
}

func NewEncoder(version uint16) *Encoder {
	if version == 0 {
		version = Version3
	}
	return &Encoder{version: version}
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	switch frame := msg.(type) {
	case DataFrame:
		return e.writeDataFrame(ctx, frame)
	case *DataFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return e.writeDataFrame(ctx, *frame)
	case SynStreamFrame:
		return e.writeSynStreamFrame(ctx, frame)
	case *SynStreamFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return e.writeSynStreamFrame(ctx, *frame)
	case SynReplyFrame:
		return e.writeSynReplyFrame(ctx, frame)
	case *SynReplyFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return e.writeSynReplyFrame(ctx, *frame)
	case RSTStreamFrame:
		return e.writeRSTStreamFrame(ctx, frame)
	case SettingsFrame:
		return e.writeSettingsFrame(ctx, frame)
	case PingFrame:
		return e.writePingFrame(ctx, frame)
	case GoAwayFrame:
		return e.writeGoAwayFrame(ctx, frame)
	case HeadersFrame:
		return e.writeHeadersFrame(ctx, frame)
	case *HeadersFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return e.writeHeadersFrame(ctx, *frame)
	case WindowUpdateFrame:
		return e.writeWindowUpdateFrame(ctx, frame)
	case UnknownFrame:
		return e.writeUnknownFrame(ctx, frame)
	case *UnknownFrame:
		if frame == nil {
			return ctx.Write(msg)
		}
		return e.writeUnknownFrame(ctx, *frame)
	default:
		return ctx.Write(msg)
	}
}

func (e *Encoder) writeDataFrame(ctx *channel.HandlerContext, frame DataFrame) error {
	if frame.StreamID == 0 || frame.StreamID > 0x7fffffff {
		frame.Release()
		return ErrInvalidFrame
	}
	length := readable(frame.Data)
	header, err := acquireHeader(ctx, headerSize)
	if err != nil {
		frame.Release()
		return err
	}
	putUint32(header, frame.StreamID&0x7fffffff)
	header[4] = frame.Flags
	putMedium(header[5:], uint32(length))
	return writeHeaderAndPayload(ctx, header, frame.Data)
}

func (e *Encoder) writeSynStreamFrame(ctx *channel.HandlerContext, frame SynStreamFrame) error {
	if frame.StreamID == 0 || frame.StreamID > 0x7fffffff || frame.AssociatedToStreamID > 0x7fffffff || frame.Priority > 7 {
		frame.Release()
		return ErrInvalidFrame
	}
	length := 10 + readable(frame.HeaderBlock)
	header, err := acquireHeader(ctx, headerSize+10)
	if err != nil {
		frame.Release()
		return err
	}
	e.putControlHeader(header, FrameTypeSynStream, frame.Flags, length)
	putUint32(header[8:], frame.StreamID&0x7fffffff)
	putUint32(header[12:], frame.AssociatedToStreamID&0x7fffffff)
	header[16] = frame.Priority << 5
	return writeHeaderAndPayload(ctx, header, frame.HeaderBlock)
}

func (e *Encoder) writeSynReplyFrame(ctx *channel.HandlerContext, frame SynReplyFrame) error {
	if frame.StreamID == 0 || frame.StreamID > 0x7fffffff {
		frame.Release()
		return ErrInvalidFrame
	}
	length := 4 + readable(frame.HeaderBlock)
	header, err := acquireHeader(ctx, headerSize+4)
	if err != nil {
		frame.Release()
		return err
	}
	e.putControlHeader(header, FrameTypeSynReply, frame.Flags, length)
	putUint32(header[8:], frame.StreamID&0x7fffffff)
	return writeHeaderAndPayload(ctx, header, frame.HeaderBlock)
}

func (e *Encoder) writeRSTStreamFrame(ctx *channel.HandlerContext, frame RSTStreamFrame) error {
	if frame.StreamID == 0 || frame.StatusCode == 0 || frame.StreamID > 0x7fffffff {
		return ErrInvalidFrame
	}
	return e.writeFixed(ctx, FrameTypeRSTStream, 0, 8, func(dst []byte) {
		putUint32(dst, frame.StreamID&0x7fffffff)
		putUint32(dst[4:], frame.StatusCode)
	})
}

func (e *Encoder) writeSettingsFrame(ctx *channel.HandlerContext, frame SettingsFrame) error {
	if len(frame.Settings) > defaultSettings {
		return ErrTooManySettings
	}
	length := 4 + len(frame.Settings)*8
	header, err := acquireHeader(ctx, headerSize+length)
	if err != nil {
		return err
	}
	e.putControlHeader(header, FrameTypeSettings, frame.Flags, length)
	putUint32(header[8:], uint32(len(frame.Settings)))
	offset := 12
	for _, setting := range frame.Settings {
		if setting.ID == 0 || setting.ID > 0x00ffffff {
			return ErrInvalidFrame
		}
		header[offset] = setting.Flags
		putMedium(header[offset+1:], setting.ID)
		putUint32(header[offset+4:], setting.Value)
		offset += 8
	}
	return writeBytes(ctx, header)
}

func (e *Encoder) writePingFrame(ctx *channel.HandlerContext, frame PingFrame) error {
	return e.writeFixed(ctx, FrameTypePing, 0, 4, func(dst []byte) {
		putUint32(dst, frame.ID)
	})
}

func (e *Encoder) writeGoAwayFrame(ctx *channel.HandlerContext, frame GoAwayFrame) error {
	if frame.LastGoodStreamID > 0x7fffffff {
		return ErrInvalidFrame
	}
	return e.writeFixed(ctx, FrameTypeGoAway, 0, 8, func(dst []byte) {
		putUint32(dst, frame.LastGoodStreamID&0x7fffffff)
		putUint32(dst[4:], frame.StatusCode)
	})
}

func (e *Encoder) writeHeadersFrame(ctx *channel.HandlerContext, frame HeadersFrame) error {
	if frame.StreamID == 0 || frame.StreamID > 0x7fffffff {
		frame.Release()
		return ErrInvalidFrame
	}
	length := 4 + readable(frame.HeaderBlock)
	header, err := acquireHeader(ctx, headerSize+4)
	if err != nil {
		frame.Release()
		return err
	}
	e.putControlHeader(header, FrameTypeHeaders, frame.Flags, length)
	putUint32(header[8:], frame.StreamID&0x7fffffff)
	return writeHeaderAndPayload(ctx, header, frame.HeaderBlock)
}

func (e *Encoder) writeWindowUpdateFrame(ctx *channel.HandlerContext, frame WindowUpdateFrame) error {
	if frame.StreamID > 0x7fffffff || frame.DeltaWindowSize == 0 || frame.DeltaWindowSize > 0x7fffffff {
		return ErrInvalidFrame
	}
	return e.writeFixed(ctx, FrameTypeWindowUpdate, 0, 8, func(dst []byte) {
		putUint32(dst, frame.StreamID&0x7fffffff)
		putUint32(dst[4:], frame.DeltaWindowSize&0x7fffffff)
	})
}

func (e *Encoder) writeUnknownFrame(ctx *channel.HandlerContext, frame UnknownFrame) error {
	length := readable(frame.Payload)
	header, err := acquireHeader(ctx, headerSize)
	if err != nil {
		frame.Release()
		return err
	}
	version := frame.Version
	if version == 0 {
		version = e.version
	}
	putUint16(header, 0x8000|version)
	putUint16(header[2:], uint16(frame.Type))
	header[4] = frame.Flags
	putMedium(header[5:], uint32(length))
	return writeHeaderAndPayload(ctx, header, frame.Payload)
}

func (e *Encoder) writeFixed(ctx *channel.HandlerContext, frameType FrameType, flags byte, length int, fill func([]byte)) error {
	header, err := acquireHeader(ctx, headerSize+length)
	if err != nil {
		return err
	}
	e.putControlHeader(header, frameType, flags, length)
	fill(header[headerSize:])
	return writeBytes(ctx, header)
}

func (e *Encoder) putControlHeader(dst []byte, frameType FrameType, flags byte, length int) {
	putUint16(dst, 0x8000|e.version)
	putUint16(dst[2:], uint16(frameType))
	dst[4] = flags
	putMedium(dst[5:], uint32(length))
}

func acquireHeader(ctx *channel.HandlerContext, size int) ([]byte, error) {
	if size < headerSize || size-headerSize > maxPayloadLength {
		return nil, ErrInvalidHeaderLength
	}
	return make([]byte, size), nil
}

func writeHeaderAndPayload(ctx *channel.HandlerContext, header []byte, payload buffer.ByteBuf) error {
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
	return codec.WriteOutboundBuffer(ctx, payload)
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
	message.Release(msg)
}

func readStreamID(in *buffer.CompositeByteBuf, index int) (uint32, error) {
	v, err := readUint32(in, index)
	if err != nil {
		return 0, err
	}
	return v & 0x7fffffff, nil
}

func readUint32(in *buffer.CompositeByteBuf, index int) (uint32, error) {
	v, err := in.ReadUnsigned(index, 4, buffer.BigEndian)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

func readMedium(in *buffer.CompositeByteBuf, index int) (uint32, error) {
	v, err := in.ReadUnsigned(index, 3, buffer.BigEndian)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

func putUint16(dst []byte, v uint16) {
	dst[0] = byte(v >> 8)
	dst[1] = byte(v)
}

func putUint32(dst []byte, v uint32) {
	dst[0] = byte(v >> 24)
	dst[1] = byte(v >> 16)
	dst[2] = byte(v >> 8)
	dst[3] = byte(v)
}

func putMedium(dst []byte, v uint32) {
	dst[0] = byte(v >> 16)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v)
}
