package http2

import (
	"bytes"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"golang.org/x/net/http2/hpack"
)

// HeaderField 是 HTTP/2 HPACK 解压后的 header 字段。
type HeaderField struct {
	Name      string
	Value     string
	Sensitive bool
}

// HeadersBlock 表示解压后的 HTTP/2 HEADERS 语义消息。
type HeadersBlock struct {
	StreamID  StreamID
	Fields    []HeaderField
	EndStream bool
	Priority  *PriorityParam
	Padding   byte
}

func (HeadersBlock) Release() {}

// PushPromiseBlock 表示解压后的 HTTP/2 PUSH_PROMISE 语义消息。
type PushPromiseBlock struct {
	StreamID         StreamID
	PromisedStreamID StreamID
	Fields           []HeaderField
	Padding          byte
}

func (PushPromiseBlock) Release() {}

// HeaderCodecConfig 描述 HPACK 编解码边界。
type HeaderCodecConfig struct {
	MaxDynamicTableSize uint32
	MaxHeaderListSize   uint32
	MaxFrameSize        int
	MaxStringLength     int
}

// HeaderDecoder 把 HTTP/2 HEADERS/PUSH_PROMISE/CONTINUATION 压缩块解码为字段。
type HeaderDecoder struct {
	decoder           *hpack.Decoder
	maxHeaderListSize uint32
	pending           *pendingHeaderBlock
}

// HeaderEncoder 把字段编码为 HPACK 压缩块并按帧大小拆分。
type HeaderEncoder struct {
	encoder      *hpack.Encoder
	buf          bytes.Buffer
	maxFrameSize int
}

type pendingHeaderKind uint8

const (
	pendingHeaders pendingHeaderKind = iota + 1
	pendingPushPromise
)

type pendingHeaderBlock struct {
	kind             pendingHeaderKind
	streamID         StreamID
	promisedStreamID StreamID
	endStream        bool
	priority         *PriorityParam
	padding          byte
	block            *buffer.CompositeByteBuf
}

// NewHeaderDecoder 创建 HTTP/2 HPACK header decoder。
func NewHeaderDecoder(cfg HeaderCodecConfig) (*HeaderDecoder, error) {
	size := cfg.MaxDynamicTableSize
	if size == 0 {
		size = 4096
	}
	decoder := hpack.NewDecoder(size, nil)
	if cfg.MaxStringLength > 0 {
		decoder.SetMaxStringLength(cfg.MaxStringLength)
	}
	return &HeaderDecoder{decoder: decoder, maxHeaderListSize: cfg.MaxHeaderListSize}, nil
}

// NewHeaderEncoder 创建 HTTP/2 HPACK header encoder。
func NewHeaderEncoder(cfg HeaderCodecConfig) (*HeaderEncoder, error) {
	maxFrameSize := cfg.MaxFrameSize
	if maxFrameSize <= 0 {
		maxFrameSize = DefaultMaxFrameSize
	}
	if maxFrameSize > MaxFrameSizeLimit {
		return nil, ErrFrameTooLarge
	}
	e := &HeaderEncoder{maxFrameSize: maxFrameSize}
	e.encoder = hpack.NewEncoder(&e.buf)
	if cfg.MaxDynamicTableSize > 0 {
		e.encoder.SetMaxDynamicTableSize(cfg.MaxDynamicTableSize)
	}
	return e, nil
}

// ChannelRead 解码完整 header block；跨 CONTINUATION 的 block 会先聚合。
func (d *HeaderDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	var (
		out any
		err error
	)
	switch frame := msg.(type) {
	case HeadersFrame:
		out, err = d.readHeaders(frame)
	case PushPromiseFrame:
		out, err = d.readPushPromise(frame)
	case ContinuationFrame:
		out, err = d.readContinuation(frame)
	default:
		ctx.FireChannelRead(msg)
		return
	}
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if out != nil {
		ctx.FireChannelRead(out)
	}
}

// Write 把解压后的 header 字段编码为 HTTP/2 typed frames。
func (e *HeaderEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	switch block := msg.(type) {
	case HeadersBlock:
		return e.writeHeaders(ctx, block)
	case PushPromiseBlock:
		return e.writePushPromise(ctx, block)
	default:
		return ctx.Write(msg)
	}
}

func (d *HeaderDecoder) readHeaders(frame HeadersFrame) (any, error) {
	if !frame.StreamID.Valid() {
		frame.Release()
		return nil, ErrInvalidStreamID
	}
	if frame.Flags&FlagEndHeaders == 0 {
		return d.startPending(pendingHeaderBlock{
			kind:      pendingHeaders,
			streamID:  frame.StreamID,
			endStream: frame.Flags&FlagEndStream != 0,
			priority:  clonePriority(frame.Priority),
			padding:   frame.Padding,
			block:     compositeFrom(frame.HeaderBlock),
		})
	}
	fields, err := d.decodeFields(frame.HeaderBlock)
	frame.Release()
	if err != nil {
		return nil, err
	}
	return HeadersBlock{StreamID: frame.StreamID, Fields: fields, EndStream: frame.Flags&FlagEndStream != 0, Priority: frame.Priority, Padding: frame.Padding}, nil
}

func (d *HeaderDecoder) readPushPromise(frame PushPromiseFrame) (any, error) {
	if !frame.StreamID.Valid() || !frame.PromisedStreamID.Valid() {
		frame.Release()
		return nil, ErrInvalidStreamID
	}
	if frame.Flags&FlagEndHeaders == 0 {
		return d.startPending(pendingHeaderBlock{
			kind:             pendingPushPromise,
			streamID:         frame.StreamID,
			promisedStreamID: frame.PromisedStreamID,
			padding:          frame.Padding,
			block:            compositeFrom(frame.HeaderBlock),
		})
	}
	fields, err := d.decodeFields(frame.HeaderBlock)
	frame.Release()
	if err != nil {
		return nil, err
	}
	return PushPromiseBlock{StreamID: frame.StreamID, PromisedStreamID: frame.PromisedStreamID, Fields: fields, Padding: frame.Padding}, nil
}

func (d *HeaderDecoder) readContinuation(frame ContinuationFrame) (any, error) {
	if d.pending == nil || d.pending.streamID != frame.StreamID {
		frame.Release()
		return nil, ErrHeaderBlock
	}
	if frame.HeaderBlock != nil {
		d.pending.block.Append(frame.HeaderBlock)
	}
	if frame.Flags&FlagEndHeaders == 0 {
		return nil, nil
	}
	pending := d.pending
	d.pending = nil
	fields, err := d.decodeFields(pending.block)
	pending.block.Release()
	if err != nil {
		return nil, err
	}
	switch pending.kind {
	case pendingHeaders:
		return HeadersBlock{StreamID: pending.streamID, Fields: fields, EndStream: pending.endStream, Priority: pending.priority, Padding: pending.padding}, nil
	case pendingPushPromise:
		return PushPromiseBlock{StreamID: pending.streamID, PromisedStreamID: pending.promisedStreamID, Fields: fields, Padding: pending.padding}, nil
	default:
		return nil, ErrHeaderBlock
	}
}

func (d *HeaderDecoder) startPending(pending pendingHeaderBlock) (any, error) {
	if d.pending != nil {
		if pending.block != nil {
			pending.block.Release()
		}
		return nil, ErrHeaderBlock
	}
	if pending.block == nil {
		pending.block = buffer.NewCompositeByteBuf()
	}
	d.pending = &pending
	return nil, nil
}

func (d *HeaderDecoder) decodeFields(block buffer.ByteBuf) ([]HeaderField, error) {
	if block == nil {
		return nil, nil
	}
	fields, err := d.decoder.DecodeFull(block.Bytes())
	if err != nil {
		return nil, err
	}
	out := make([]HeaderField, 0, len(fields))
	var size uint32
	for _, field := range fields {
		size += uint32(len(field.Name) + len(field.Value) + 32)
		if d.maxHeaderListSize > 0 && size > d.maxHeaderListSize {
			return nil, ErrHeaderListTooLarge
		}
		out = append(out, HeaderField{Name: field.Name, Value: field.Value, Sensitive: field.Sensitive})
	}
	return out, nil
}

func (e *HeaderEncoder) writeHeaders(ctx *channel.HandlerContext, block HeadersBlock) error {
	if !block.StreamID.Valid() {
		return ErrInvalidStreamID
	}
	encoded, err := e.encodeFields(block.Fields)
	if err != nil {
		return err
	}
	flags := Flags(0)
	if block.EndStream {
		flags |= FlagEndStream
	}
	firstOverhead := 0
	if block.Priority != nil {
		firstOverhead += 5
	}
	if block.Padding > 0 {
		firstOverhead += int(block.Padding) + 1
	}
	return e.writeHeaderFrames(ctx, encoded, firstOverhead, func(part buffer.ByteBuf, endHeaders bool) TypedFrame {
		partFlags := flags
		if endHeaders {
			partFlags |= FlagEndHeaders
		}
		return HeadersFrame{StreamID: block.StreamID, Flags: partFlags, HeaderBlock: part, Priority: block.Priority, Padding: block.Padding}
	}, block.StreamID)
}

func (e *HeaderEncoder) writePushPromise(ctx *channel.HandlerContext, block PushPromiseBlock) error {
	if !block.StreamID.Valid() || !block.PromisedStreamID.Valid() {
		return ErrInvalidStreamID
	}
	encoded, err := e.encodeFields(block.Fields)
	if err != nil {
		return err
	}
	firstOverhead := 4
	if block.Padding > 0 {
		firstOverhead += int(block.Padding) + 1
	}
	return e.writeHeaderFrames(ctx, encoded, firstOverhead, func(part buffer.ByteBuf, endHeaders bool) TypedFrame {
		flags := Flags(0)
		if endHeaders {
			flags = FlagEndHeaders
		}
		return PushPromiseFrame{StreamID: block.StreamID, PromisedStreamID: block.PromisedStreamID, Flags: flags, HeaderBlock: part, Padding: block.Padding}
	}, block.StreamID)
}

func (e *HeaderEncoder) encodeFields(fields []HeaderField) ([]byte, error) {
	e.buf.Reset()
	for _, field := range fields {
		if err := e.encoder.WriteField(hpack.HeaderField{Name: field.Name, Value: field.Value, Sensitive: field.Sensitive}); err != nil {
			return nil, err
		}
	}
	return append([]byte(nil), e.buf.Bytes()...), nil
}

func (e *HeaderEncoder) writeHeaderFrames(ctx *channel.HandlerContext, encoded []byte, firstOverhead int, first func(buffer.ByteBuf, bool) TypedFrame, streamID StreamID) error {
	firstLimit := e.maxFrameSize - firstOverhead
	if firstLimit < 0 {
		return ErrFrameTooLarge
	}
	if firstLimit == 0 && len(encoded) > 0 {
		return ErrFrameTooLarge
	}
	if len(encoded) <= firstLimit {
		part, err := bufferFromBytes(ctx, encoded)
		if err != nil {
			return err
		}
		return writeTypedFrame(ctx, first(part, true))
	}
	part, err := bufferFromBytes(ctx, encoded[:firstLimit])
	if err != nil {
		return err
	}
	if err := writeTypedFrame(ctx, first(part, false)); err != nil {
		return err
	}
	for offset := firstLimit; offset < len(encoded); {
		next := offset + e.maxFrameSize
		if next > len(encoded) {
			next = len(encoded)
		}
		end := next == len(encoded)
		part, err := bufferFromBytes(ctx, encoded[offset:next])
		if err != nil {
			return err
		}
		frame := ContinuationFrame{StreamID: streamID, HeaderBlock: part}
		if end {
			frame.Flags = FlagEndHeaders
		}
		if err := writeTypedFrame(ctx, frame); err != nil {
			return err
		}
		offset = next
	}
	return nil
}

func writeTypedFrame(ctx *channel.HandlerContext, frame TypedFrame) error {
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func bufferFromBytes(ctx *channel.HandlerContext, data []byte) (buffer.ByteBuf, error) {
	if len(data) == 0 {
		return nil, nil
	}
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func compositeFrom(block buffer.ByteBuf) *buffer.CompositeByteBuf {
	comp := buffer.NewCompositeByteBuf()
	if block != nil {
		comp.Append(block)
	}
	return comp
}

func clonePriority(priority *PriorityParam) *PriorityParam {
	if priority == nil {
		return nil
	}
	cloned := *priority
	return &cloned
}
