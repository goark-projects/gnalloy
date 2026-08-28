package flow

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/internal/message"
)

// MessageSize 返回入站流控队列使用的消息字节数。
func MessageSize(msg any) int {
	switch v := msg.(type) {
	case nil:
		return 0
	case buffer.ByteBuf:
		return v.ReadableBytes()
	case []byte:
		return len(v)
	case string:
		return len(v)
	case interface{ ReadableBytes() int }:
		return v.ReadableBytes()
	default:
		return 0
	}
}

func releaseMessage(msg any) {
	message.Release(msg)
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
