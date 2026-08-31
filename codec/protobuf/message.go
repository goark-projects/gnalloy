package protobuf

import (
	"reflect"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"

	"google.golang.org/protobuf/proto"
)

// DefaultMaxMessageSize 是 protobuf 对象解码默认最大消息大小。
const DefaultMaxMessageSize = 4 << 20

// MessageFactory 创建单次解码使用的 protobuf 目标对象。
type MessageFactory func() proto.Message

// EncoderConfig 描述 protobuf 对象出站编码参数。
type EncoderConfig struct {
	Marshal proto.MarshalOptions
}

// DecoderConfig 描述 protobuf 对象入站解码参数。
type DecoderConfig struct {
	NewMessage     MessageFactory
	MaxMessageSize int
	Unmarshal      proto.UnmarshalOptions
}

// Encoder 把 proto.Message 编码为 ByteBuf。
type Encoder struct {
	options proto.MarshalOptions
}

// ProtobufEncoder 是 Netty 命名风格的 Encoder 别名。
type ProtobufEncoder = Encoder

// NewEncoder 创建默认 protobuf 对象编码器。
func NewEncoder() *Encoder {
	return NewEncoderWithConfig(EncoderConfig{})
}

// NewEncoderWithConfig 创建可配置 protobuf 对象编码器。
func NewEncoderWithConfig(cfg EncoderConfig) *Encoder {
	return &Encoder{options: cfg.Marshal}
}

// NewProtobufEncoder 创建 Netty 命名风格的 protobuf 对象编码器。
func NewProtobufEncoder() *ProtobufEncoder {
	return NewEncoder()
}

// Write 编码 proto.Message；非 protobuf 消息直接透传。
func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	pm, ok := msg.(proto.Message)
	if !ok {
		return ctx.Write(msg)
	}
	if isNilProtoMessage(pm) {
		return codec.ErrInvalidEncoder
	}
	size := e.encodedSize(pm)
	out, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return err
	}
	if err := e.encode(pm, out); err != nil {
		out.Release()
		return err
	}
	return codec.WriteOutboundBuffer(ctx, out)
}

func (e *Encoder) encodedSize(pm proto.Message) int {
	size := proto.Size(pm)
	if size == 0 {
		return 1
	}
	return size
}

func (e *Encoder) encode(pm proto.Message, out buffer.ByteBuf) error {
	view := out.WritableBytesView()
	encoded, err := e.options.MarshalAppend(view[:0], pm)
	if err != nil {
		return err
	}
	if len(encoded) > len(view) {
		return buffer.ErrNoWritableBytes
	}
	if len(encoded) > 0 && len(view) > 0 && &encoded[0] != &view[0] {
		copy(view, encoded)
	}
	return out.AdvanceWriter(len(encoded))
}

// Decoder 把 ByteBuf 解码为 proto.Message。
type Decoder struct {
	*codec.MessageToMessageDecoder
	cfg DecoderConfig
}

// ProtobufDecoder 是 Netty 命名风格的 Decoder 别名。
type ProtobufDecoder = Decoder

// NewDecoder 创建 protobuf 对象解码器。
func NewDecoder(factory MessageFactory, maxMessageSize int) (*Decoder, error) {
	return NewDecoderWithConfig(DecoderConfig{NewMessage: factory, MaxMessageSize: maxMessageSize})
}

// NewDecoderWithConfig 创建可配置 protobuf 对象解码器。
func NewDecoderWithConfig(cfg DecoderConfig) (*Decoder, error) {
	cfg = normalizeDecoderConfig(cfg)
	if cfg.NewMessage == nil || cfg.MaxMessageSize <= 0 {
		return nil, ErrInvalidConfig
	}
	d := &Decoder{cfg: cfg}
	d.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(d)
	return d, nil
}

// NewProtobufDecoder 创建 Netty 命名风格的 protobuf 对象解码器。
func NewProtobufDecoder(factory MessageFactory, maxMessageSize int) (*ProtobufDecoder, error) {
	return NewDecoder(factory, maxMessageSize)
}

func normalizeDecoderConfig(cfg DecoderConfig) DecoderConfig {
	if cfg.MaxMessageSize == 0 {
		cfg.MaxMessageSize = DefaultMaxMessageSize
	}
	return cfg
}

func (d *Decoder) AcceptInboundMessage(msg any) bool {
	_, ok := msg.(buffer.ByteBuf)
	return ok
}

func (d *Decoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	src := msg.(buffer.ByteBuf)
	size := src.ReadableBytes()
	if size < 0 {
		return buffer.ErrInvalidIndex
	}
	if size > d.cfg.MaxMessageSize {
		return codec.NewTooLongFrameError(size, d.cfg.MaxMessageSize, 0)
	}
	dst := d.cfg.NewMessage()
	if isNilProtoMessage(dst) {
		return ErrInvalidConfig
	}
	data, err := readableMessageBytes(src, size)
	if err != nil {
		return err
	}
	if err := d.cfg.Unmarshal.Unmarshal(data, dst); err != nil {
		return err
	}
	out.Add(dst)
	return nil
}

func readableMessageBytes(src buffer.ByteBuf, size int) ([]byte, error) {
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		return data, nil
	}
	data := make([]byte, size)
	if buffer.CopyReadableBytes(data, src) != size {
		return nil, buffer.ErrNotEnoughBytes
	}
	return data, nil
}

func isNilProtoMessage(msg proto.Message) bool {
	if msg == nil {
		return true
	}
	v := reflect.ValueOf(msg)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
