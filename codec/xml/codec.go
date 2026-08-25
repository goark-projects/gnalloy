package xml

import (
	"bytes"
	stdxml "encoding/xml"
	"errors"
	"io"

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
	data := in.Bytes()
	offset, ok, err := completeDocumentLength(data)
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
	decoder := stdxml.NewDecoder(bytes.NewReader(buf.Bytes()))
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
	decoder := stdxml.NewDecoder(bytes.NewReader(data))
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

func isUnexpectedEOF(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
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
