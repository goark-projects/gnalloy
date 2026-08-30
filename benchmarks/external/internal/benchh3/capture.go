package benchh3

import (
	"bytes"
	"fmt"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	codechttp3 "goark.dev/gnalloy/codec/http3"
)

type responseCapture struct {
	status   string
	expected []byte
	reply    []byte
	err      error
}

func (c *responseCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case codechttp3.HeadersBlock:
		for _, field := range frame.Fields {
			if field.Name == ":status" {
				c.status = field.Value
			}
		}
	case codechttp3.DataFrame:
		c.copyData(frame.Data)
		frame.Release()
	default:
		releaseMessage(msg)
	}
}

func (c *responseCapture) copyData(src buffer.ByteBuf) {
	if src == nil || c.err != nil {
		return
	}
	var stack [8][]byte
	for _, segment := range src.ReadableSlices(stack[:0]) {
		if len(segment) > len(c.expected)-len(c.reply) {
			c.err = fmt.Errorf("benchh3: response body too large")
			return
		}
		c.reply = append(c.reply, segment...)
	}
}

func (c *responseCapture) complete() bool {
	return c.status == "200" && len(c.reply) == len(c.expected) && bytes.Equal(c.reply, c.expected)
}

func releaseMessage(msg any) {
	if releaser, ok := msg.(interface{ Release() }); ok {
		releaser.Release()
	}
}
