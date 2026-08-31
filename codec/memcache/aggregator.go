package memcache

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// ObjectAggregator 把完整 binary frame 转换为 request/response 对象。
type ObjectAggregator struct {
	*codec.MessageToMessageDecoder
	maxContentLength int
}

// NewObjectAggregator 创建完整对象聚合器。
func NewObjectAggregator(maxContentLength int) (*ObjectAggregator, error) {
	if maxContentLength <= 0 {
		return nil, ErrInvalidConfig
	}
	a := &ObjectAggregator{maxContentLength: maxContentLength}
	a.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(a)
	return a, nil
}

func (a *ObjectAggregator) AcceptInboundMessage(msg any) bool {
	_, ok := frameFromMessage(msg)
	return ok
}

func (a *ObjectAggregator) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	frame, _ := frameFromMessage(msg)
	if frame.BodyLength() > a.maxContentLength {
		return codec.NewTooLongFrameError(frame.BodyLength(), a.maxContentLength, 0)
	}
	switch frame.Magic {
	case MagicRequest:
		out.Add(requestFromFrame(frame))
	case MagicResponse:
		out.Add(responseFromFrame(frame))
	default:
		return ErrInvalidFrame
	}
	return nil
}
