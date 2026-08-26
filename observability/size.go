package observability

// MessageSizer 估算消息占用的可观测字节数。
//
// 返回负数会被 handler/metrics 当作 0 处理。估算器不拥有消息生命周期，不能
// Retain 或 Release 入站/出站对象。
type MessageSizer interface {
	MessageSize(msg any) int64
}

// MessageSizerFunc 允许用函数实现 MessageSizer。
type MessageSizerFunc func(msg any) int64

func (f MessageSizerFunc) MessageSize(msg any) int64 {
	if f == nil {
		return 0
	}
	return f(msg)
}

// ReadableBytesSizer 使用 ByteBuf 风格的 ReadableBytes 方法估算消息大小。
var ReadableBytesSizer MessageSizer = MessageSizerFunc(func(msg any) int64 {
	sized, ok := msg.(interface{ ReadableBytes() int })
	if !ok {
		return 0
	}
	n := sized.ReadableBytes()
	if n <= 0 {
		return 0
	}
	return int64(n)
})

func NormalizeMessageSizer(sizer MessageSizer) MessageSizer {
	if sizer == nil {
		return ReadableBytesSizer
	}
	return sizer
}

func NormalizeChannelRecorder(recorder ChannelRecorder) ChannelRecorder {
	if recorder == nil {
		return NoopChannelRecorder{}
	}
	return recorder
}
