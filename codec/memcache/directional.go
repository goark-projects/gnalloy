package memcache

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type clientInboundDecoder struct{}

func (clientInboundDecoder) AcceptInboundMessage(msg any) bool {
	_, ok := frameFromMessage(msg)
	return ok
}

func (clientInboundDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	frame, _ := frameFromMessage(msg)
	if frame.Magic != MagicResponse {
		return ErrInvalidFrame
	}
	out.Add(responseFromFrame(frame))
	return nil
}

type clientOutboundEncoder struct{}

func (clientOutboundEncoder) AcceptOutboundMessage(msg any) bool {
	_, ok := requestFromMessage(msg)
	return ok
}

func (clientOutboundEncoder) Encode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	request, _ := requestFromMessage(msg)
	if !request.Valid() {
		return ErrInvalidFrame
	}
	out.Add(request.toFrame())
	return nil
}

type serverInboundDecoder struct{}

func (serverInboundDecoder) AcceptInboundMessage(msg any) bool {
	_, ok := frameFromMessage(msg)
	return ok
}

func (serverInboundDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	frame, _ := frameFromMessage(msg)
	if frame.Magic != MagicRequest {
		return ErrInvalidFrame
	}
	out.Add(requestFromFrame(frame))
	return nil
}

type serverOutboundEncoder struct{}

func (serverOutboundEncoder) AcceptOutboundMessage(msg any) bool {
	_, ok := responseFromMessage(msg)
	return ok
}

func (serverOutboundEncoder) Encode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	response, _ := responseFromMessage(msg)
	if !response.Valid() {
		return ErrInvalidFrame
	}
	out.Add(response.toFrame())
	return nil
}

func requestFromFrame(frame Frame) Request {
	return Request{
		Opcode:   frame.Opcode,
		DataType: frame.DataType,
		VBucket:  frame.VBucket,
		Opaque:   frame.Opaque,
		CAS:      frame.CAS,
		Extras:   retainPart(frame.Extras),
		Key:      retainPart(frame.Key),
		Value:    retainPart(frame.Value),
	}
}

func responseFromFrame(frame Frame) Response {
	return Response{
		Opcode:   frame.Opcode,
		DataType: frame.DataType,
		Status:   frame.Status,
		Opaque:   frame.Opaque,
		CAS:      frame.CAS,
		Extras:   retainPart(frame.Extras),
		Key:      retainPart(frame.Key),
		Value:    retainPart(frame.Value),
	}
}

func frameFromMessage(msg any) (Frame, bool) {
	switch v := msg.(type) {
	case Frame:
		return v, true
	case *Frame:
		if v != nil {
			return *v, true
		}
	}
	return Frame{}, false
}

func requestFromMessage(msg any) (Request, bool) {
	switch v := msg.(type) {
	case Request:
		return v, true
	case *Request:
		if v != nil {
			return *v, true
		}
	}
	return Request{}, false
}

func responseFromMessage(msg any) (Response, bool) {
	switch v := msg.(type) {
	case Response:
		return v, true
	case *Response:
		if v != nil {
			return *v, true
		}
	}
	return Response{}, false
}
