package smtp

import (
	"strconv"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const defaultMaxLineLength = 1024

type ResponseDecoder struct {
	*codec.ByteToMessageDecoder
	maxLineLength int
	code          int
	details       []string
}

func NewResponseDecoder(maxLineLength int) (*ResponseDecoder, error) {
	if maxLineLength <= 0 {
		maxLineLength = defaultMaxLineLength
	}
	d := &ResponseDecoder{maxLineLength: maxLineLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *ResponseDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	lineEnd, ok := findLF(in, in.ReaderIndex())
	if !ok {
		if in.ReadableBytes() > d.maxLineLength {
			return nil, codec.ErrFrameTooLong
		}
		return nil, nil
	}
	lineLen := lineEnd - in.ReaderIndex()
	if lineLen > d.maxLineLength {
		return nil, codec.ErrFrameTooLong
	}
	line, err := lineString(in, in.ReaderIndex(), lineEnd)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(lineEnd + 1 - in.ReaderIndex()); err != nil {
		return nil, err
	}
	code, more, detail, err := parseResponseLine(line)
	if err != nil {
		d.reset()
		return nil, err
	}
	if d.code != 0 && d.code != code {
		d.reset()
		return nil, ErrInvalidResponse
	}
	if d.code == 0 {
		d.code = code
	}
	if detail != "" {
		d.details = append(d.details, detail)
	}
	if more {
		return nil, nil
	}
	resp := Response{Code: d.code, Details: append([]string(nil), d.details...)}
	d.reset()
	return resp, nil
}

func (d *ResponseDecoder) reset() {
	d.code = 0
	for i := range d.details {
		d.details[i] = ""
	}
	d.details = d.details[:0]
}

type RequestEncoder struct{}

func NewRequestEncoder() *RequestEncoder {
	return &RequestEncoder{}
}

func (e *RequestEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	req, ok := msg.(Request)
	if !ok {
		if ptr, ptrOK := msg.(*Request); ptrOK && ptr != nil {
			req = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	line, err := commandLine(req)
	if err != nil {
		return err
	}
	return writeString(ctx, line)
}

type ResponseEncoder struct{}

func NewResponseEncoder() *ResponseEncoder {
	return &ResponseEncoder{}
}

func (e *ResponseEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	resp, ok := msg.(Response)
	if !ok {
		if ptr, ptrOK := msg.(*Response); ptrOK && ptr != nil {
			resp = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	lines, err := responseLines(resp)
	if err != nil {
		return err
	}
	total := 0
	for _, line := range lines {
		total += len(line)
	}
	out, err := ctx.Channel().Allocator().Acquire(total)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := out.WriteBytes([]byte(line)); err != nil {
			out.Release()
			return err
		}
	}
	return ctx.Write(out)
}

type DataEncoder struct{}

func NewDataEncoder() *DataEncoder {
	return &DataEncoder{}
}

func (e *DataEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	data, ok := msg.(Data)
	if !ok {
		if ptr, ptrOK := msg.(*Data); ptrOK && ptr != nil {
			data = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	if data.Payload != nil {
		out, err := dotStuff(ctx.Channel().Allocator(), data.Payload)
		data.Payload.Release()
		if err != nil {
			return err
		}
		if out.ReadableBytes() > 0 {
			if err := ctx.Write(out); err != nil {
				out.Release()
				return err
			}
		} else {
			out.Release()
		}
	}
	if data.Last {
		return writeString(ctx, "\r\n.\r\n")
	}
	return nil
}

func parseResponseLine(line string) (int, bool, string, error) {
	if len(line) < 3 {
		return 0, false, "", ErrInvalidLine
	}
	code, err := strconv.Atoi(line[:3])
	if err != nil || code < 100 || code > 999 {
		return 0, false, "", ErrInvalidLine
	}
	if len(line) == 3 {
		return code, false, "", nil
	}
	switch line[3] {
	case '-':
		return code, true, detail(line), nil
	case ' ':
		return code, false, detail(line), nil
	default:
		return 0, false, "", ErrInvalidLine
	}
}

func detail(line string) string {
	if len(line) <= 4 {
		return ""
	}
	return line[4:]
}

func dotStuff(alloc buffer.Allocator, in buffer.ByteBuf) (buffer.ByteBuf, error) {
	extra := 0
	atLineStart := true
	for _, b := range in.Bytes() {
		if atLineStart && b == '.' {
			extra++
		}
		atLineStart = b == '\n'
	}
	out, err := alloc.Acquire(in.ReadableBytes() + extra)
	if err != nil {
		return nil, err
	}
	atLineStart = true
	for _, b := range in.Bytes() {
		if atLineStart && b == '.' {
			if _, err := out.WriteBytes([]byte{'.'}); err != nil {
				out.Release()
				return nil, err
			}
		}
		if _, err := out.WriteBytes([]byte{b}); err != nil {
			out.Release()
			return nil, err
		}
		atLineStart = b == '\n'
	}
	return out, nil
}

func findLF(in *buffer.CompositeByteBuf, start int) (int, bool) {
	for i := start; i < in.WriterIndex(); i++ {
		b, ok := in.GetByte(i)
		if ok && b == '\n' {
			return i, true
		}
	}
	return 0, false
}

func lineString(in *buffer.CompositeByteBuf, start int, end int) (string, error) {
	if end > start {
		if b, _ := in.GetByte(end - 1); b == '\r' {
			end--
		}
	}
	part, err := in.Slice(start, end-start)
	if err != nil {
		return "", err
	}
	defer part.Release()
	return string(part.Bytes()), nil
}

func writeString(ctx *channel.HandlerContext, text string) error {
	out, err := ctx.Channel().Allocator().Acquire(len(text))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes([]byte(text)); err != nil {
		out.Release()
		return err
	}
	return ctx.Write(out)
}
