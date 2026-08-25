package redis

import (
	"strconv"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type Value interface {
	RESPType() byte
	Release()
}

type SimpleString struct{ Value string }
type Error struct{ Value string }
type Integer struct{ Value int64 }

type BulkString struct {
	Data buffer.ByteBuf
	Null bool
}

type Array struct {
	Values []Value
	Null   bool
}

func (SimpleString) RESPType() byte { return '+' }
func (SimpleString) Release()       {}
func (Error) RESPType() byte        { return '-' }
func (Error) Release()              {}
func (Integer) RESPType() byte      { return ':' }
func (Integer) Release()            {}
func (BulkString) RESPType() byte   { return '$' }

func (v BulkString) Release() {
	if v.Data != nil {
		v.Data.Release()
	}
}

func (Array) RESPType() byte { return '*' }

func (v Array) Release() {
	for _, item := range v.Values {
		if item != nil {
			item.Release()
		}
	}
}

type ValueDecoder struct {
	*codec.MessageToMessageDecoder
}

func NewValueDecoder() *ValueDecoder {
	d := &ValueDecoder{}
	d.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(d)
	return d
}

func (d *ValueDecoder) AcceptInboundMessage(msg any) bool {
	_, ok := msg.(buffer.ByteBuf)
	return ok
}

func (d *ValueDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	frame := msg.(buffer.ByteBuf)
	value, total, err := decodeValueAt(frame, frame.ReaderIndex())
	if err != nil {
		if value != nil {
			value.Release()
		}
		return err
	}
	if frame.ReaderIndex()+total != frame.WriterIndex() {
		value.Release()
		return codec.ErrInvalidFrameLength
	}
	out.Add(value)
	return nil
}

type ValueEncoder struct{}

func NewValueEncoder() *ValueEncoder {
	return &ValueEncoder{}
}

func (e *ValueEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	value, ok := msg.(Value)
	if !ok {
		return ctx.Write(msg)
	}
	return writeValue(ctx, value)
}

func decodeValueAt(buf buffer.ByteBuf, index int) (Value, int, error) {
	prefix, ok := buf.GetByte(index)
	if !ok {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	line, lineEnd, err := readRESPLine(buf, index+1)
	if err != nil {
		return nil, 0, err
	}
	lineLength := lineEnd - index + 2
	switch prefix {
	case '+':
		return SimpleString{Value: line}, lineLength, nil
	case '-':
		return Error{Value: line}, lineLength, nil
	case ':':
		n, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return nil, 0, codec.ErrInvalidFrameLength
		}
		return Integer{Value: n}, lineLength, nil
	case '$':
		return decodeBulkString(buf, index, line, lineLength)
	case '*':
		return decodeArray(buf, index, line, lineLength)
	default:
		return nil, 0, codec.ErrInvalidFrameLength
	}
}

func decodeBulkString(buf buffer.ByteBuf, index int, line string, lineLength int) (Value, int, error) {
	n, err := strconv.Atoi(line)
	if err != nil {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	if n < 0 {
		return BulkString{Null: true}, lineLength, nil
	}
	payloadIndex := index + lineLength
	if payloadIndex+n+2 > buf.WriterIndex() {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	cr, _ := buf.GetByte(payloadIndex + n)
	lf, _ := buf.GetByte(payloadIndex + n + 1)
	if cr != '\r' || lf != '\n' {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	payload, err := buf.Slice(payloadIndex, n)
	if err != nil {
		return nil, 0, err
	}
	return BulkString{Data: payload}, lineLength + n + 2, nil
}

func decodeArray(buf buffer.ByteBuf, index int, line string, lineLength int) (Value, int, error) {
	count, err := strconv.Atoi(line)
	if err != nil {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	if count < 0 {
		return Array{Null: true}, lineLength, nil
	}
	values := make([]Value, 0, count)
	total := lineLength
	for i := 0; i < count; i++ {
		value, n, err := decodeValueAt(buf, index+total)
		if err != nil {
			for _, item := range values {
				item.Release()
			}
			return nil, 0, err
		}
		values = append(values, value)
		total += n
	}
	return Array{Values: values}, total, nil
}

func readRESPLine(buf buffer.ByteBuf, start int) (string, int, error) {
	for i := start; i+1 < buf.WriterIndex(); i++ {
		cr, _ := buf.GetByte(i)
		if cr != '\r' {
			continue
		}
		lf, _ := buf.GetByte(i + 1)
		if lf != '\n' {
			continue
		}
		part, err := buf.Slice(start, i-start)
		if err != nil {
			return "", 0, err
		}
		defer part.Release()
		return string(part.Bytes()), i, nil
	}
	return "", 0, codec.ErrInvalidFrameLength
}

func writeValue(ctx *channel.HandlerContext, value Value) error {
	switch v := value.(type) {
	case SimpleString:
		return writeASCII(ctx, "+"+v.Value+"\r\n")
	case Error:
		return writeASCII(ctx, "-"+v.Value+"\r\n")
	case Integer:
		return writeASCII(ctx, ":"+strconv.FormatInt(v.Value, 10)+"\r\n")
	case BulkString:
		return writeBulkString(ctx, v)
	case Array:
		return writeArray(ctx, v)
	default:
		value.Release()
		return codec.ErrInvalidFrameLength
	}
}

func writeArray(ctx *channel.HandlerContext, value Array) error {
	if value.Null {
		return writeASCII(ctx, "*-1\r\n")
	}
	if err := writeASCII(ctx, "*"+strconv.Itoa(len(value.Values))+"\r\n"); err != nil {
		value.Release()
		return err
	}
	for i, item := range value.Values {
		if err := writeValue(ctx, item); err != nil {
			for _, rest := range value.Values[i+1:] {
				if rest != nil {
					rest.Release()
				}
			}
			return err
		}
	}
	return nil
}

func writeBulkString(ctx *channel.HandlerContext, value BulkString) error {
	if value.Null {
		return writeASCII(ctx, "$-1\r\n")
	}
	length := 0
	if value.Data != nil {
		length = value.Data.ReadableBytes()
	}
	if err := writeASCII(ctx, "$"+strconv.Itoa(length)+"\r\n"); err != nil {
		value.Release()
		return err
	}
	if value.Data != nil {
		if length == 0 {
			value.Data.Release()
		} else if err := ctx.Write(value.Data); err != nil {
			value.Data.Release()
			return err
		}
	}
	return writeASCII(ctx, "\r\n")
}

func writeASCII(ctx *channel.HandlerContext, data string) error {
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes([]byte(data)); err != nil {
		out.Release()
		return err
	}
	return codec.WriteOutboundBuffer(ctx, out)
}
