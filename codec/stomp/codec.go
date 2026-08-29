package stomp

import (
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	defaultMaxHeaderBytes = 8192
	defaultMaxBodyBytes   = 8 << 20
)

type Decoder struct {
	*codec.ByteToMessageDecoder
	maxHeaderBytes int
	maxBodyBytes   int
}

func NewDecoder(maxHeaderBytes int, maxBodyBytes int) (*Decoder, error) {
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = defaultMaxHeaderBytes
	}
	if maxBodyBytes < 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	d := &Decoder{maxHeaderBytes: maxHeaderBytes, maxBodyBytes: maxBodyBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *Decoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	reader := in.ReaderIndex()
	if in.ReadableBytes() == 0 {
		return nil, nil
	}
	first, ok := in.GetByte(reader)
	if !ok {
		return nil, nil
	}
	if first == '\n' {
		if err := in.SkipBytes(1); err != nil {
			return nil, err
		}
		return Heartbeat(), nil
	}
	commandEnd, ok := findLF(in, reader)
	if !ok {
		if in.ReadableBytes() > d.maxHeaderBytes {
			return nil, codec.ErrFrameTooLong
		}
		return nil, nil
	}
	command, err := lineString(in, reader, commandEnd)
	if err != nil {
		return nil, err
	}
	if command == "" {
		return nil, ErrInvalidFrame
	}
	headers, bodyStart, ok, err := readHeaders(in, commandEnd+1, d.maxHeaderBytes)
	if err != nil || !ok {
		return nil, err
	}
	headerBytes := bodyStart - reader
	if headerBytes > d.maxHeaderBytes {
		return nil, codec.ErrFrameTooLong
	}
	contentLength, hasContentLength, err := headers.ContentLength()
	if err != nil {
		return nil, err
	}
	if hasContentLength {
		return d.decodeContentLengthFrame(in, reader, bodyStart, Command(command), headers, contentLength)
	}
	return d.decodeNulTerminatedFrame(in, reader, bodyStart, Command(command), headers)
}

func (d *Decoder) decodeContentLengthFrame(in *buffer.CompositeByteBuf, reader int, bodyStart int, command Command, headers Headers, contentLength int) (any, error) {
	if contentLength > d.maxBodyBytes {
		return nil, codec.ErrFrameTooLong
	}
	nulIndex := bodyStart + contentLength
	if in.WriterIndex() <= nulIndex {
		return nil, nil
	}
	nul, ok := in.GetByte(nulIndex)
	if !ok || nul != 0 {
		return nil, ErrInvalidFrame
	}
	body, err := sliceBody(in, bodyStart, contentLength)
	if err != nil {
		return nil, err
	}
	total := consumeFrameEnd(in, reader, nulIndex+1)
	if err := in.SkipBytes(total); err != nil {
		if body != nil {
			body.Release()
		}
		return nil, err
	}
	return Frame{Command: command, Headers: headers, Body: body}, nil
}

func (d *Decoder) decodeNulTerminatedFrame(in *buffer.CompositeByteBuf, reader int, bodyStart int, command Command, headers Headers) (any, error) {
	nulIndex, ok := findNUL(in, bodyStart)
	if !ok {
		if in.WriterIndex()-bodyStart > d.maxBodyBytes {
			return nil, codec.ErrFrameTooLong
		}
		return nil, nil
	}
	bodyLength := nulIndex - bodyStart
	if bodyLength > d.maxBodyBytes {
		return nil, codec.ErrFrameTooLong
	}
	body, err := sliceBody(in, bodyStart, bodyLength)
	if err != nil {
		return nil, err
	}
	total := consumeFrameEnd(in, reader, nulIndex+1)
	if err := in.SkipBytes(total); err != nil {
		if body != nil {
			body.Release()
		}
		return nil, err
	}
	return Frame{Command: command, Headers: headers, Body: body}, nil
}

type Encoder struct{}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(Frame)
	if !ok {
		if ptr, ptrOK := msg.(*Frame); ptrOK && ptr != nil {
			frame = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	if frame.Heartbeat {
		frame.Release()
		return writeSmall(ctx, []byte{'\n'})
	}
	if frame.Command == "" {
		frame.Release()
		return ErrInvalidFrame
	}
	bodyLen := 0
	if frame.Body != nil {
		bodyLen = frame.Body.ReadableBytes()
	}
	hasContentLength := frame.Headers.Has("content-length")
	headerSize := encodedHeaderSize(frame, bodyLen, hasContentLength)
	header, err := ctx.Channel().Allocator().Acquire(headerSize)
	if err != nil {
		frame.Release()
		return err
	}
	if err := writeHeader(header, frame, bodyLen, hasContentLength); err != nil {
		header.Release()
		frame.Release()
		return err
	}
	if err := ctx.Write(header); err != nil {
		header.Release()
		frame.Release()
		return err
	}
	if frame.Body != nil {
		if err := ctx.Write(frame.Body); err != nil {
			frame.Body.Release()
			return err
		}
		frame.Body = nil
	}
	return writeSmall(ctx, []byte{0})
}

func readHeaders(in *buffer.CompositeByteBuf, start int, maxHeaderBytes int) (Headers, int, bool, error) {
	var headers Headers
	index := start
	for {
		lineEnd, ok := findLF(in, index)
		if !ok {
			if in.WriterIndex()-start > maxHeaderBytes {
				return nil, 0, false, codec.ErrFrameTooLong
			}
			return nil, 0, false, nil
		}
		line, err := lineString(in, index, lineEnd)
		if err != nil {
			return nil, 0, false, err
		}
		index = lineEnd + 1
		if line == "" {
			return headers, index, true, nil
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || name == "" {
			return nil, 0, false, ErrInvalidHeader
		}
		headers.Add(unescape(name), unescape(value))
	}
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

func findNUL(in *buffer.CompositeByteBuf, start int) (int, bool) {
	for i := start; i < in.WriterIndex(); i++ {
		b, ok := in.GetByte(i)
		if ok && b == 0 {
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
	if end < start {
		return "", ErrInvalidFrame
	}
	part, err := in.Slice(start, end-start)
	if err != nil {
		return "", err
	}
	defer part.Release()
	return string(part.Bytes()), nil
}

func sliceBody(in *buffer.CompositeByteBuf, index int, length int) (buffer.ByteBuf, error) {
	if length == 0 {
		return nil, nil
	}
	return in.Slice(index, length)
}

func consumeFrameEnd(in *buffer.CompositeByteBuf, reader int, index int) int {
	if b, ok := in.GetByte(index); ok && b == '\n' {
		return index + 1 - reader
	}
	if b, ok := in.GetByte(index); ok && b == '\r' {
		if next, nextOK := in.GetByte(index + 1); nextOK && next == '\n' {
			return index + 2 - reader
		}
	}
	return index - reader
}

func writeSmall(ctx *channel.HandlerContext, data []byte) error {
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

func unescape(src string) string {
	if !strings.Contains(src, "\\") {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		if src[i] != '\\' || i+1 >= len(src) {
			b.WriteByte(src[i])
			continue
		}
		i++
		switch src[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 'c':
			b.WriteByte(':')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte(src[i])
		}
	}
	return b.String()
}

func escapedLen(src string) int {
	n := len(src)
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n', '\r', ':', '\\':
			n++
		}
	}
	return n
}

func appendEscaped(dst []byte, src string) []byte {
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case ':':
			dst = append(dst, '\\', 'c')
		case '\\':
			dst = append(dst, '\\', '\\')
		default:
			dst = append(dst, src[i])
		}
	}
	return dst
}
