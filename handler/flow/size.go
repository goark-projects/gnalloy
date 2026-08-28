package flow

import "goark.dev/gnalloy/buffer"

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
	if msg == nil {
		return
	}
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() bool }); ok {
		releasable.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
