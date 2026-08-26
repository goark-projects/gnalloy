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

// ChannelOptionAssignment 保存一个类型安全 ChannelOption 赋值。
type ChannelOptionAssignment struct {
	name  optionKeyID
	value any
}

// Assignment 把选项和值绑定成可复用的装配项。
func (o ChannelOption[T]) Assignment(value T) ChannelOptionAssignment {
	return ChannelOptionAssignment{name: o.name, value: value}
}

type ChannelOptions struct {
	attrs AttributeMap
}

func NewChannelOptions() *ChannelOptions {
	return &ChannelOptions{}
}

// Clone 复制当前选项集合。选项值按接口引用复制，不深拷贝业务对象。
func (o *ChannelOptions) Clone() *ChannelOptions {
	out := NewChannelOptions()
	o.CopyTo(out)
	return out
}

// Apply 写入一组选项装配项，后出现的同名选项覆盖前值。
func (o *ChannelOptions) Apply(assignments ...ChannelOptionAssignment) {
	if o == nil {
		return
	}
	for _, assignment := range assignments {
		if assignment.name == "" {
			continue
		}
		o.set(assignment.name, assignment.value)
	}
}

// CopyTo 把当前选项复制到目标集合，用于 Bootstrap 配置快照落到 Channel。
func (o *ChannelOptions) CopyTo(dst *ChannelOptions) {
	if o == nil || dst == nil {
		return
	}
	values := o.attrs.snapshot()
	for key, value := range values {
		dst.set(optionKeyID(key), value)
	}
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
