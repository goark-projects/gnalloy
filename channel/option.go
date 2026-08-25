package channel

import "goark.dev/gnalloy/transport"

type optionKeyID string

// ChannelOption 是 Netty ChannelOption 的 Go 化类型安全版本。
type ChannelOption[T any] struct {
	name         optionKeyID
	defaultValue T
}

var (
	OptionAutoRead             = NewChannelOption("AUTO_READ", true)
	OptionReadBufferSize       = NewChannelOption("READ_BUFFER_SIZE", 0)
	OptionWriteBufferWatermark = NewChannelOption("WRITE_BUFFER_WATERMARK", transport.DefaultWriteBufferWatermark())
)

func NewChannelOption[T any](name string, defaultValue T) ChannelOption[T] {
	return ChannelOption[T]{name: optionKeyID(name), defaultValue: defaultValue}
}

func (o ChannelOption[T]) Name() string {
	return string(o.name)
}

func (o ChannelOption[T]) Default() T {
	return o.defaultValue
}

func (o ChannelOption[T]) Get(options *ChannelOptions) T {
	if options == nil {
		return o.defaultValue
	}
	value, ok := options.get(o.name)
	if !ok {
		return o.defaultValue
	}
	typed, ok := value.(T)
	if !ok {
		return o.defaultValue
	}
	return typed
}

func (o ChannelOption[T]) Set(options *ChannelOptions, value T) {
	if options == nil {
		return
	}
	options.set(o.name, value)
}

func (o ChannelOption[T]) Remove(options *ChannelOptions) {
	if options == nil {
		return
	}
	options.remove(o.name)
}

type ChannelOptions struct {
	attrs AttributeMap
}

func NewChannelOptions() *ChannelOptions {
	return &ChannelOptions{}
}

func (o *ChannelOptions) get(key optionKeyID) (any, bool) {
	return o.attrs.get(attributeKeyID(key))
}

func (o *ChannelOptions) set(key optionKeyID, value any) {
	o.attrs.set(attributeKeyID(key), value)
}

func (o *ChannelOptions) remove(key optionKeyID) {
	o.attrs.remove(attributeKeyID(key))
}
