package xml

import (
	"bytes"
	stdxml "encoding/xml"
	"errors"
	"io"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameLength int
}

func NewFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	if maxFrameLength <= 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &FrameDecoder{maxFrameLength: maxFrameLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *FrameDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() == 0 {
		return nil, nil
	}
	if in.ReadableBytes() > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
	}
	offset, ok, err := completeDocumentLengthFromBuffer(in)
	if err != nil || !ok {
		return nil, err
	}
	frame, err := in.Slice(in.ReaderIndex(), offset)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(offset); err != nil {
		frame.Release()
		return nil, err
	}
	return frame, nil
}

type TokenDecoder struct {
	*codec.MessageToMessageDecoder
}

func NewTokenDecoder() *TokenDecoder {
	d := &TokenDecoder{}
	d.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(d)
	return d
}

func (d *TokenDecoder) AcceptInboundMessage(msg any) bool {
	_, ok := msg.(buffer.ByteBuf)
	return ok
}

func (d *TokenDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	buf := msg.(buffer.ByteBuf)
	decoder := stdxml.NewDecoder(newReadableBytesReader(buf))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		out.Add(convertToken(token))
	}
}

func completeDocumentLength(data []byte) (int, bool, error) {
	return completeDocumentLengthFromReader(bytes.NewReader(data))
}

// completeDocumentLengthFromBuffer 使用 ByteBuf 可读切片做流式扫描，避免 composite 累积区整块复制。
func completeDocumentLengthFromBuffer(buf *buffer.CompositeByteBuf) (int, bool, error) {
	return completeDocumentLengthFromReader(newReadableBytesReader(buf))
}

func completeDocumentLengthFromReader(r io.Reader) (int, bool, error) {
	decoder := stdxml.NewDecoder(r)
	depth := 0
	started := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return 0, false, nil
		}
		if err != nil {
			if isUnexpectedEOF(err) {
				return 0, false, nil
			}
			return 0, false, err
		}
		switch token.(type) {
		case stdxml.StartElement:
			depth++
			started = true
		case stdxml.EndElement:
			depth--
			if started && depth == 0 {
				return int(decoder.InputOffset()), true, nil
			}
			if depth < 0 {
				return 0, false, ErrInvalidXML
			}
		}
	}
}

func newReadableBytesReader(buf buffer.ByteBuf) io.Reader {
	var scratch [8][]byte
	slices := buf.ReadableSlices(scratch[:0])
	switch len(slices) {
	case 0:
		return bytes.NewReader(nil)
	case 1:
		return bytes.NewReader(slices[0])
	default:
		return &byteSlicesReader{slices: slices}
	}
}

// byteSlicesReader 把多个连续逻辑切片适配为 io.Reader，不推进 ByteBuf 的 reader index。
type byteSlicesReader struct {
	slices [][]byte
	index  int
	offset int
}

func (r *byteSlicesReader) Read(p []byte) (int, error) {
	written := 0
	for len(p) > written && r.index < len(r.slices) {
		current := r.slices[r.index]
		if r.offset >= len(current) {
			r.index++
			r.offset = 0
			continue
		}
		n := copy(p[written:], current[r.offset:])
		written += n
		r.offset += n
	}
	if written > 0 {
		return written, nil
	}
	return 0, io.EOF
}

func isUnexpectedEOF(err error) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var syntaxErr *stdxml.SyntaxError
	return errors.As(err, &syntaxErr) && strings.Contains(syntaxErr.Msg, "unexpected EOF")
}

func convertToken(token stdxml.Token) any {
	switch t := token.(type) {
	case stdxml.StartElement:
		attrs := make([]Attr, len(t.Attr))
		for i, attr := range t.Attr {
			attrs[i] = Attr{Name: attr.Name.Local, Space: attr.Name.Space, Value: attr.Value}
		}
		return StartElement{Name: t.Name.Local, Space: t.Name.Space, Attrs: attrs}
	case stdxml.EndElement:
		return EndElement{Name: t.Name.Local, Space: t.Name.Space}
	case stdxml.CharData:
		return CharData{Text: string(t)}
	case stdxml.Comment:
		return Comment{Text: string(t)}
	case stdxml.ProcInst:
		return ProcInst{Target: t.Target, Inst: string(t.Inst)}
	case stdxml.Directive:
		return Directive{Text: string(t)}
	default:
		return nil
	}
}
