package http3

import (
	"bytes"
	"io"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"

	"github.com/quic-go/qpack"
)

// HeaderField 是 HTTP/3 QPACK 解压后的 header 字段。
type HeaderField struct {
	Name  string
	Value string
}

// HeadersBlock 表示解压后的 HTTP/3 HEADERS 语义消息。
type HeadersBlock struct {
	Fields []HeaderField
}

// Release 保持 HeadersBlock 可进入统一的 pipeline 释放路径。
func (HeadersBlock) Release() {}

// PushPromiseBlock 表示解压后的 HTTP/3 PUSH_PROMISE 语义消息。
type PushPromiseBlock struct {
	PushID uint64
	Fields []HeaderField
}

// Release 保持 PushPromiseBlock 可进入统一的 pipeline 释放路径。
func (PushPromiseBlock) Release() {}

// HeaderCodecConfig 描述 HTTP/3 QPACK 编解码边界。
type HeaderCodecConfig struct {
	// MaxHeaderListSize 按 RFC 语义统计 name/value 加 32 字节开销，0 表示不额外限制。
	MaxHeaderListSize uint64
}

// HeaderDecoder 把 HEADERS/PUSH_PROMISE 的 QPACK block 解码为字段。
type HeaderDecoder struct {
	decoder           *qpack.Decoder
	maxHeaderListSize uint64
}

// HeaderEncoder 把字段编码为 QPACK block，再包装为 HTTP/3 frame。
type HeaderEncoder struct {
	encoder *qpack.Encoder
	buf     bytes.Buffer
}

// NewHeaderDecoder 创建 HTTP/3 QPACK header decoder。
func NewHeaderDecoder(cfg HeaderCodecConfig) *HeaderDecoder {
	return &HeaderDecoder{decoder: qpack.NewDecoder(), maxHeaderListSize: cfg.MaxHeaderListSize}
}

// NewHeaderEncoder 创建 HTTP/3 QPACK header encoder。
func NewHeaderEncoder() *HeaderEncoder {
	e := &HeaderEncoder{}
	e.encoder = qpack.NewEncoder(&e.buf)
	return e
}

// ChannelRead 解码 HEADERS 与 PUSH_PROMISE，其它消息继续透传。
func (d *HeaderDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case HeadersFrame:
		fields, err := d.decodeFields(frame.HeaderBlock)
		frame.Release()
		if err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		ctx.FireChannelRead(HeadersBlock{Fields: fields})
	case PushPromiseFrame:
		fields, err := d.decodeFields(frame.HeaderBlock)
		frame.Release()
		if err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		ctx.FireChannelRead(PushPromiseBlock{PushID: frame.PushID, Fields: fields})
	default:
		ctx.FireChannelRead(msg)
	}
}

// Write 编码 HTTP/3 header 语义消息，其它消息继续透传。
func (e *HeaderEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	switch block := msg.(type) {
	case HeadersBlock:
		headerBlock, err := e.encodeFields(ctx, block.Fields)
		if err != nil {
			return err
		}
		return writeHeaderFrame(ctx, HeadersFrame{HeaderBlock: headerBlock})
	case PushPromiseBlock:
		headerBlock, err := e.encodeFields(ctx, block.Fields)
		if err != nil {
			return err
		}
		return writeHeaderFrame(ctx, PushPromiseFrame{PushID: block.PushID, HeaderBlock: headerBlock})
	default:
		return ctx.Write(msg)
	}
}

func (d *HeaderDecoder) decodeFields(block buffer.ByteBuf) ([]HeaderField, error) {
	if block == nil {
		return nil, nil
	}
	decode := d.decoder.Decode(block.Bytes())
	fields := make([]HeaderField, 0, 8)
	var size uint64
	for {
		field, err := decode()
		if err == io.EOF {
			return fields, nil
		}
		if err != nil {
			return nil, err
		}
		size += uint64(len(field.Name) + len(field.Value) + 32)
		if d.maxHeaderListSize > 0 && size > d.maxHeaderListSize {
			return nil, ErrHeaderListTooLarge
		}
		fields = append(fields, HeaderField{Name: field.Name, Value: field.Value})
	}
}

func (e *HeaderEncoder) encodeFields(ctx *channel.HandlerContext, fields []HeaderField) (buffer.ByteBuf, error) {
	e.buf.Reset()
	if len(fields) == 0 {
		return http3BufferFromBytes(ctx, []byte{0, 0})
	}
	defer func() { _ = e.encoder.Close() }()
	for _, field := range fields {
		if err := e.encoder.WriteField(qpack.HeaderField{Name: field.Name, Value: field.Value}); err != nil {
			return nil, err
		}
	}
	return http3BufferFromBytes(ctx, e.buf.Bytes())
}

func writeHeaderFrame(ctx *channel.HandlerContext, frame any) error {
	if err := ctx.Write(frame); err != nil {
		releaseMessage(frame)
		return err
	}
	return nil
}

func http3BufferFromBytes(ctx *channel.HandlerContext, data []byte) (buffer.ByteBuf, error) {
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
