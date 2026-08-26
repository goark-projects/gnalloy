package http2

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type TypedFrame interface {
	Release()
}

type DataFrame struct {
	StreamID StreamID
	Flags    Flags
	Data     buffer.ByteBuf
	Padding  byte
}

func (f DataFrame) Release() {
	if f.Data != nil {
		f.Data.Release()
	}
}

type PriorityParam struct {
	Exclusive        bool
	StreamDependency StreamID
	Weight           byte
}

type HeadersFrame struct {
	StreamID    StreamID
	Flags       Flags
	HeaderBlock buffer.ByteBuf
	Priority    *PriorityParam
	Padding     byte
}

func (f HeadersFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type PriorityFrame struct {
	StreamID StreamID
	Priority PriorityParam
}

func (PriorityFrame) Release() {}

type RSTStreamFrame struct {
	StreamID  StreamID
	ErrorCode uint32
}

func (RSTStreamFrame) Release() {}

type Setting struct {
	ID    uint16
	Value uint32
}

type SettingsFrame struct {
	Ack      bool
	Settings []Setting
}

func (SettingsFrame) Release() {}

type PushPromiseFrame struct {
	StreamID         StreamID
	PromisedStreamID StreamID
	Flags            Flags
	HeaderBlock      buffer.ByteBuf
	Padding          byte
}

func (f PushPromiseFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type PingFrame struct {
	Ack  bool
	Data [8]byte
}

func (PingFrame) Release() {}

type GoAwayFrame struct {
	LastStreamID StreamID
	ErrorCode    uint32
	DebugData    buffer.ByteBuf
}

func (f GoAwayFrame) Release() {
	if f.DebugData != nil {
		f.DebugData.Release()
	}
}

type WindowUpdateFrame struct {
	StreamID  StreamID
	Increment uint32
}

func (WindowUpdateFrame) Release() {}

type ContinuationFrame struct {
	StreamID    StreamID
	Flags       Flags
	HeaderBlock buffer.ByteBuf
}

func (f ContinuationFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type UnknownFrame struct {
	Frame Frame
}

func (f UnknownFrame) Release() {
	f.Frame.Release()
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
	_, ok := msg.(Frame)
	return ok
}

func (d *TypedFrameDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	frame := msg.(Frame)
	typed, err := DecodeTypedFrame(frame)
	if err != nil {
		return err
	}
	out.Add(typed)
	return nil
}

func DecodeTypedFrame(frame Frame) (TypedFrame, error) {
	switch frame.Type {
	case FrameData:
		return decodeDataFrame(frame)
	case FrameHeaders:
		return decodeHeadersFrame(frame)
	case FramePriority:
		return decodePriorityFrame(frame)
	case FrameRSTStream:
		return decodeRSTStreamFrame(frame)
	case FrameSettings:
		return decodeSettingsFrame(frame)
	case FramePushPromise:
		return decodePushPromiseFrame(frame)
	case FramePing:
		return decodePingFrame(frame)
	case FrameGoAway:
		return decodeGoAwayFrame(frame)
	case FrameWindowUpdate:
		return decodeWindowUpdateFrame(frame)
	case FrameContinuation:
		return decodeContinuationFrame(frame)
	default:
		if frame.Payload != nil {
			frame.Payload.Retain()
		}
		return UnknownFrame{Frame: frame}, nil
	}
}

type TypedFrameEncoder struct{}

func NewTypedFrameEncoder() *TypedFrameEncoder {
	return &TypedFrameEncoder{}
}

func (e *TypedFrameEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok, err := EncodeTypedFrame(ctx, msg)
	if err != nil {
		return err
	}
	if !ok {
		return ctx.Write(msg)
	}
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func EncodeTypedFrame(ctx *channel.HandlerContext, msg any) (Frame, bool, error) {
	switch f := msg.(type) {
	case DataFrame:
		frame, err := encodeDataFrame(ctx, f)
		return frame, true, err
	case HeadersFrame:
		frame, err := encodeHeadersFrame(ctx, f)
		return frame, true, err
	case PriorityFrame:
		frame, err := encodePriorityFrame(ctx, f)
		return frame, true, err
	case RSTStreamFrame:
		frame, err := encodeRSTStreamFrame(ctx, f)
		return frame, true, err
	case SettingsFrame:
		frame, err := encodeSettingsFrame(ctx, f)
		return frame, true, err
	case PushPromiseFrame:
		frame, err := encodePushPromiseFrame(ctx, f)
		return frame, true, err
	case PingFrame:
		frame, err := encodePingFrame(ctx, f)
		return frame, true, err
	case GoAwayFrame:
		frame, err := encodeGoAwayFrame(ctx, f)
		return frame, true, err
	case WindowUpdateFrame:
		frame, err := encodeWindowUpdateFrame(ctx, f)
		return frame, true, err
	case ContinuationFrame:
		frame, err := encodeContinuationFrame(ctx, f)
		return frame, true, err
	case UnknownFrame:
		return f.Frame, true, nil
	default:
		return Frame{}, false, nil
	}
}

func decodeDataFrame(frame Frame) (DataFrame, error) {
	if !frame.StreamID.Valid() {
		return DataFrame{}, ErrInvalidStreamID
	}
	index, length, padding, err := payloadRange(frame, frame.Flags&FlagPadded != 0)
	if err != nil {
		return DataFrame{}, err
	}
	data, err := slice(frame.Payload, index, length)
	if err != nil {
		return DataFrame{}, err
	}
	return DataFrame{StreamID: frame.StreamID, Flags: frame.Flags, Data: data, Padding: padding}, nil
}

func decodeHeadersFrame(frame Frame) (HeadersFrame, error) {
	if !frame.StreamID.Valid() {
		return HeadersFrame{}, ErrInvalidStreamID
	}
	index, length, padding, err := payloadRange(frame, frame.Flags&FlagPadded != 0)
	if err != nil {
		return HeadersFrame{}, err
	}
	var priority *PriorityParam
	if frame.Flags&FlagPriority != 0 {
		if length < 5 {
			return HeadersFrame{}, ErrInvalidFrame
		}
		p, err := readPriority(frame.Payload, index)
		if err != nil {
			return HeadersFrame{}, err
		}
		priority = &p
		index += 5
		length -= 5
	}
	block, err := slice(frame.Payload, index, length)
	if err != nil {
		return HeadersFrame{}, err
	}
	return HeadersFrame{StreamID: frame.StreamID, Flags: frame.Flags, HeaderBlock: block, Priority: priority, Padding: padding}, nil
}

func decodePriorityFrame(frame Frame) (PriorityFrame, error) {
	if !frame.StreamID.Valid() || readable(frame.Payload) != 5 {
		return PriorityFrame{}, ErrInvalidFrame
	}
	priority, err := readPriority(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil {
		return PriorityFrame{}, err
	}
	return PriorityFrame{StreamID: frame.StreamID, Priority: priority}, nil
}

func decodeRSTStreamFrame(frame Frame) (RSTStreamFrame, error) {
	if !frame.StreamID.Valid() || readable(frame.Payload) != 4 {
		return RSTStreamFrame{}, ErrInvalidFrame
	}
	code, err := readUint32(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil {
		return RSTStreamFrame{}, err
	}
	return RSTStreamFrame{StreamID: frame.StreamID, ErrorCode: code}, nil
}

func decodeSettingsFrame(frame Frame) (SettingsFrame, error) {
	if frame.StreamID != 0 || readable(frame.Payload)%6 != 0 {
		return SettingsFrame{}, ErrInvalidFrame
	}
	if frame.Flags&FlagAck != 0 {
		if readable(frame.Payload) != 0 {
			return SettingsFrame{}, ErrInvalidFrame
		}
		return SettingsFrame{Ack: true}, nil
	}
	count := readable(frame.Payload) / 6
	if count == 0 {
		return SettingsFrame{}, nil
	}
	settings := make([]Setting, count)
	index := frame.Payload.ReaderIndex()
	for i := range settings {
		id, err := readUint16(frame.Payload, index)
		if err != nil {
			return SettingsFrame{}, err
		}
		value, err := readUint32(frame.Payload, index+2)
		if err != nil {
			return SettingsFrame{}, err
		}
		settings[i] = Setting{ID: id, Value: value}
		index += 6
	}
	return SettingsFrame{Settings: settings}, nil
}

func decodePushPromiseFrame(frame Frame) (PushPromiseFrame, error) {
	if !frame.StreamID.Valid() {
		return PushPromiseFrame{}, ErrInvalidStreamID
	}
	index, length, padding, err := payloadRange(frame, frame.Flags&FlagPadded != 0)
	if err != nil {
		return PushPromiseFrame{}, err
	}
	if length < 4 {
		return PushPromiseFrame{}, ErrInvalidFrame
	}
	promised, err := readStreamID(frame.Payload, index)
	if err != nil {
		return PushPromiseFrame{}, err
	}
	if !promised.Valid() {
		return PushPromiseFrame{}, ErrInvalidStreamID
	}
	block, err := slice(frame.Payload, index+4, length-4)
	if err != nil {
		return PushPromiseFrame{}, err
	}
	return PushPromiseFrame{StreamID: frame.StreamID, PromisedStreamID: promised, Flags: frame.Flags, HeaderBlock: block, Padding: padding}, nil
}

func decodePingFrame(frame Frame) (PingFrame, error) {
	if frame.StreamID != 0 || readable(frame.Payload) != 8 {
		return PingFrame{}, ErrInvalidFrame
	}
	var data [8]byte
	for i := range data {
		b, _ := frame.Payload.GetByte(frame.Payload.ReaderIndex() + i)
		data[i] = b
	}
	return PingFrame{Ack: frame.Flags&FlagAck != 0, Data: data}, nil
}

func decodeGoAwayFrame(frame Frame) (GoAwayFrame, error) {
	if frame.StreamID != 0 || readable(frame.Payload) < 8 {
		return GoAwayFrame{}, ErrInvalidFrame
	}
	lastID, err := readStreamID(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil {
		return GoAwayFrame{}, err
	}
	code, err := readUint32(frame.Payload, frame.Payload.ReaderIndex()+4)
	if err != nil {
		return GoAwayFrame{}, err
	}
	debug, err := slice(frame.Payload, frame.Payload.ReaderIndex()+8, readable(frame.Payload)-8)
	if err != nil {
		return GoAwayFrame{}, err
	}
	return GoAwayFrame{LastStreamID: lastID, ErrorCode: code, DebugData: debug}, nil
}

func decodeWindowUpdateFrame(frame Frame) (WindowUpdateFrame, error) {
	if readable(frame.Payload) != 4 {
		return WindowUpdateFrame{}, ErrInvalidFrame
	}
	increment, err := readStreamID(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil {
		return WindowUpdateFrame{}, err
	}
	if increment == 0 {
		return WindowUpdateFrame{}, ErrInvalidFrame
	}
	return WindowUpdateFrame{StreamID: frame.StreamID, Increment: uint32(increment)}, nil
}

func decodeContinuationFrame(frame Frame) (ContinuationFrame, error) {
	if !frame.StreamID.Valid() {
		return ContinuationFrame{}, ErrInvalidStreamID
	}
	block, err := slice(frame.Payload, readerIndex(frame.Payload), readable(frame.Payload))
	if err != nil {
		return ContinuationFrame{}, err
	}
	return ContinuationFrame{StreamID: frame.StreamID, Flags: frame.Flags, HeaderBlock: block}, nil
}

func encodeDataFrame(ctx *channel.HandlerContext, frame DataFrame) (Frame, error) {
	if !frame.StreamID.Valid() {
		frame.Release()
		return Frame{}, ErrInvalidStreamID
	}
	payload, flags, err := encodePaddedPayload(ctx, frame.Data, frame.Padding)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: FrameData, Flags: frame.Flags | flags, StreamID: frame.StreamID, Payload: payload}, nil
}

func encodeHeadersFrame(ctx *channel.HandlerContext, frame HeadersFrame) (Frame, error) {
	if !frame.StreamID.Valid() {
		frame.Release()
		return Frame{}, ErrInvalidStreamID
	}
	payload := frame.HeaderBlock
	flags := frame.Flags
	if frame.Priority != nil {
		prefix, err := priorityPayload(ctx, *frame.Priority)
		if err != nil {
			frame.Release()
			return Frame{}, err
		}
		payload = concat(prefix, payload)
		flags |= FlagPriority
	}
	payload, padFlags, err := encodePaddedPayload(ctx, payload, frame.Padding)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: FrameHeaders, Flags: flags | padFlags, StreamID: frame.StreamID, Payload: payload}, nil
}

func encodePriorityFrame(ctx *channel.HandlerContext, frame PriorityFrame) (Frame, error) {
	if !frame.StreamID.Valid() {
		return Frame{}, ErrInvalidStreamID
	}
	payload, err := priorityPayload(ctx, frame.Priority)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: FramePriority, StreamID: frame.StreamID, Payload: payload}, nil
}

func encodeRSTStreamFrame(ctx *channel.HandlerContext, frame RSTStreamFrame) (Frame, error) {
	if !frame.StreamID.Valid() {
		return Frame{}, ErrInvalidStreamID
	}
	payload, err := fixedPayload(ctx, 4)
	if err != nil {
		return Frame{}, err
	}
	putUint32(payload.Bytes(), frame.ErrorCode)
	return Frame{Type: FrameRSTStream, StreamID: frame.StreamID, Payload: payload}, nil
}

func encodeSettingsFrame(ctx *channel.HandlerContext, frame SettingsFrame) (Frame, error) {
	if frame.Ack {
		return SettingsAck(), nil
	}
	payload, err := fixedPayload(ctx, len(frame.Settings)*6)
	if err != nil {
		return Frame{}, err
	}
	data := payload.Bytes()
	for i, setting := range frame.Settings {
		offset := i * 6
		putUint16(data[offset:], setting.ID)
		putUint32(data[offset+2:], setting.Value)
	}
	return Frame{Type: FrameSettings, Payload: payload}, nil
}

func encodePushPromiseFrame(ctx *channel.HandlerContext, frame PushPromiseFrame) (Frame, error) {
	if !frame.StreamID.Valid() || !frame.PromisedStreamID.Valid() {
		frame.Release()
		return Frame{}, ErrInvalidStreamID
	}
	prefix, err := fixedPayload(ctx, 4)
	if err != nil {
		frame.Release()
		return Frame{}, err
	}
	putUint32(prefix.Bytes(), uint32(frame.PromisedStreamID)&0x7fffffff)
	payload, flags, err := encodePaddedPayload(ctx, concat(prefix, frame.HeaderBlock), frame.Padding)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: FramePushPromise, Flags: frame.Flags | flags, StreamID: frame.StreamID, Payload: payload}, nil
}

func encodePingFrame(ctx *channel.HandlerContext, frame PingFrame) (Frame, error) {
	payload, err := fixedPayload(ctx, 8)
	if err != nil {
		return Frame{}, err
	}
	copy(payload.Bytes(), frame.Data[:])
	flags := Flags(0)
	if frame.Ack {
		flags = FlagAck
	}
	return Frame{Type: FramePing, Flags: flags, Payload: payload}, nil
}

func encodeGoAwayFrame(ctx *channel.HandlerContext, frame GoAwayFrame) (Frame, error) {
	prefix, err := fixedPayload(ctx, 8)
	if err != nil {
		frame.Release()
		return Frame{}, err
	}
	putUint32(prefix.Bytes(), uint32(frame.LastStreamID)&0x7fffffff)
	putUint32(prefix.Bytes()[4:], frame.ErrorCode)
	return Frame{Type: FrameGoAway, Payload: concat(prefix, frame.DebugData)}, nil
}

func encodeWindowUpdateFrame(ctx *channel.HandlerContext, frame WindowUpdateFrame) (Frame, error) {
	if frame.StreamID > maxStreamID || frame.Increment == 0 || frame.Increment > uint32(maxStreamID) {
		return Frame{}, ErrInvalidFrame
	}
	payload, err := fixedPayload(ctx, 4)
	if err != nil {
		return Frame{}, err
	}
	putUint32(payload.Bytes(), frame.Increment&0x7fffffff)
	return Frame{Type: FrameWindowUpdate, StreamID: frame.StreamID, Payload: payload}, nil
}

func encodeContinuationFrame(_ *channel.HandlerContext, frame ContinuationFrame) (Frame, error) {
	if !frame.StreamID.Valid() {
		frame.Release()
		return Frame{}, ErrInvalidStreamID
	}
	return Frame{Type: FrameContinuation, Flags: frame.Flags, StreamID: frame.StreamID, Payload: frame.HeaderBlock}, nil
}

func payloadRange(frame Frame, padded bool) (int, int, byte, error) {
	if readable(frame.Payload) == 0 {
		if padded {
			return 0, 0, 0, ErrInvalidFrame
		}
		return 0, 0, 0, nil
	}
	index := frame.Payload.ReaderIndex()
	length := readable(frame.Payload)
	padding := byte(0)
	if padded {
		padding, _ = frame.Payload.GetByte(index)
		index++
		length--
		if int(padding) > length {
			return 0, 0, 0, ErrInvalidFrame
		}
		length -= int(padding)
	}
	return index, length, padding, nil
}

func encodePaddedPayload(ctx *channel.HandlerContext, payload buffer.ByteBuf, padding byte) (buffer.ByteBuf, Flags, error) {
	if padding == 0 {
		return payload, 0, nil
	}
	prefix, err := fixedPayload(ctx, 1)
	if err != nil {
		if payload != nil {
			payload.Release()
		}
		return nil, 0, err
	}
	if _, err := prefix.WriteBytes([]byte{padding}); err != nil {
		prefix.Release()
		if payload != nil {
			payload.Release()
		}
		return nil, 0, err
	}
	pad, err := fixedPayload(ctx, int(padding))
	if err != nil {
		prefix.Release()
		if payload != nil {
			payload.Release()
		}
		return nil, 0, err
	}
	return concat(concat(prefix, payload), pad), FlagPadded, nil
}

func priorityPayload(ctx *channel.HandlerContext, priority PriorityParam) (buffer.ByteBuf, error) {
	payload, err := fixedPayload(ctx, 5)
	if err != nil {
		return nil, err
	}
	value := uint32(priority.StreamDependency) & 0x7fffffff
	if priority.Exclusive {
		value |= 0x80000000
	}
	putUint32(payload.Bytes(), value)
	payload.Bytes()[4] = priority.Weight
	return payload, nil
}

func fixedPayload(ctx *channel.HandlerContext, size int) (buffer.ByteBuf, error) {
	payload, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return nil, err
	}
	if size > 0 {
		if err := payload.AdvanceWriter(size); err != nil {
			payload.Release()
			return nil, err
		}
	}
	return payload, nil
}

func concat(first buffer.ByteBuf, second buffer.ByteBuf) buffer.ByteBuf {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	out := buffer.NewCompositeByteBuf()
	out.Append(first)
	out.Append(second)
	return out
}

func slice(buf buffer.ByteBuf, index int, length int) (buffer.ByteBuf, error) {
	if length == 0 {
		return nil, nil
	}
	return buf.Slice(index, length)
}

func readable(buf buffer.ByteBuf) int {
	if buf == nil {
		return 0
	}
	return buf.ReadableBytes()
}

func readerIndex(buf buffer.ByteBuf) int {
	if buf == nil {
		return 0
	}
	return buf.ReaderIndex()
}

func readPriority(buf buffer.ByteBuf, index int) (PriorityParam, error) {
	raw, err := readUint32(buf, index)
	if err != nil {
		return PriorityParam{}, err
	}
	weight, ok := buf.GetByte(index + 4)
	if !ok {
		return PriorityParam{}, ErrInvalidFrame
	}
	return PriorityParam{Exclusive: raw&0x80000000 != 0, StreamDependency: StreamID(raw & 0x7fffffff), Weight: weight}, nil
}

func readStreamID(buf buffer.ByteBuf, index int) (StreamID, error) {
	raw, err := readUint32(buf, index)
	if err != nil {
		return 0, err
	}
	return StreamID(raw & 0x7fffffff), nil
}

func readUint16(buf buffer.ByteBuf, index int) (uint16, error) {
	if index+2 > buf.WriterIndex() {
		return 0, ErrInvalidFrame
	}
	hi, ok := buf.GetByte(index)
	if !ok {
		return 0, ErrInvalidFrame
	}
	lo, ok := buf.GetByte(index + 1)
	if !ok {
		return 0, ErrInvalidFrame
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

func readUint32(buf buffer.ByteBuf, index int) (uint32, error) {
	if index+4 > buf.WriterIndex() {
		return 0, ErrInvalidFrame
	}
	var value uint32
	for i := 0; i < 4; i++ {
		b, ok := buf.GetByte(index + i)
		if !ok {
			return 0, ErrInvalidFrame
		}
		value = (value << 8) | uint32(b)
	}
	return value, nil
}

func putUint16(dst []byte, value uint16) {
	dst[0] = byte(value >> 8)
	dst[1] = byte(value)
}

func putUint32(dst []byte, value uint32) {
	dst[0] = byte(value >> 24)
	dst[1] = byte(value >> 16)
	dst[2] = byte(value >> 8)
	dst[3] = byte(value)
}
