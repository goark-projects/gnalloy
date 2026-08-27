package recipes

import (
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	// HandlerNameByteBufEcho 是 ByteBuf echo handler 的默认名称。
	HandlerNameByteBufEcho = "bytebuf-echo"
	// HandlerNameLengthFieldDecoder 是 length-field 入站解码器的默认名称。
	HandlerNameLengthFieldDecoder = "length-field-decoder"
	// HandlerNameLengthFieldPrepender 是 length-field 出站编码器的默认名称。
	HandlerNameLengthFieldPrepender = "length-field-prepender"
	// HandlerNameLineFrameDecoder 是 line-frame 入站解码器的默认名称。
	HandlerNameLineFrameDecoder = "line-frame-decoder"
	// HandlerNameStringDecoder 是 string 入站解码器的默认名称。
	HandlerNameStringDecoder = "string-decoder"
	// HandlerNameStringEncoder 是 string 出站编码器的默认名称。
	HandlerNameStringEncoder = "string-encoder"
)

// LengthFieldConfig 描述常见 length-field frame pipeline。
type LengthFieldConfig struct {
	MaxFrameLength     int
	LengthFieldOffset  int
	LengthFieldLength  int
	LengthAdjustment   int
	InitialBytesToSkip int
	ByteOrder          buffer.ByteOrder
	FailSlow           bool
}

// LineConfig 描述 line-based 文本 pipeline。
type LineConfig struct {
	MaxLength     int
	KeepDelimiter bool
	FailSlow      bool
	StringCodec   bool
}

// ByteBufEcho 创建零复制 ByteBuf echo pipeline。
func ByteBufEcho() bootstrap.ChildInitializer {
	return Initializer(Use(HandlerNameByteBufEcho, byteBufEchoHandler{}))
}

// LengthFieldFrames 创建 4 字节 big-endian 长度字段 frame pipeline。
func LengthFieldFrames(cfg LengthFieldConfig, app ...HandlerSpec) bootstrap.ChildInitializer {
	cfg = normalizeLengthFieldConfig(cfg)
	base := []HandlerSpec{
		UseFactory(HandlerNameLengthFieldDecoder, func() (channel.Handler, error) {
			return codec.NewLengthFieldBasedFrameDecoderWithOptions(
				cfg.MaxFrameLength,
				cfg.LengthFieldOffset,
				cfg.LengthFieldLength,
				cfg.LengthAdjustment,
				cfg.InitialBytesToSkip,
				cfg.ByteOrder,
				!cfg.FailSlow,
			)
		}),
		UseFactory(HandlerNameLengthFieldPrepender, func() (channel.Handler, error) {
			return codec.NewLengthFieldPrepender(cfg.LengthFieldLength, cfg.ByteOrder)
		}),
	}
	return Initializer(appendSpecs(base, app)...)
}

// LineFrames 创建 line-based frame pipeline，可选 string 编解码。
func LineFrames(cfg LineConfig, app ...HandlerSpec) bootstrap.ChildInitializer {
	cfg = normalizeLineConfig(cfg)
	base := []HandlerSpec{
		UseFactory(HandlerNameLineFrameDecoder, func() (channel.Handler, error) {
			return codec.NewLineBasedFrameDecoderWithOptions(cfg.MaxLength, !cfg.KeepDelimiter, !cfg.FailSlow)
		}),
	}
	if cfg.StringCodec {
		base = append(base,
			UseFactory(HandlerNameStringDecoder, func() (channel.Handler, error) {
				return codec.NewStringDecoder(), nil
			}),
			UseFactory(HandlerNameStringEncoder, func() (channel.Handler, error) {
				return codec.NewStringEncoder(), nil
			}),
		)
	}
	return Initializer(appendSpecs(base, app)...)
}

func normalizeLengthFieldConfig(cfg LengthFieldConfig) LengthFieldConfig {
	if cfg.MaxFrameLength <= 0 {
		cfg.MaxFrameLength = 1024 * 1024
	}
	if cfg.LengthFieldLength == 0 {
		cfg.LengthFieldLength = 4
	}
	if cfg.InitialBytesToSkip == 0 {
		cfg.InitialBytesToSkip = cfg.LengthFieldOffset + cfg.LengthFieldLength
	}
	return cfg
}

func normalizeLineConfig(cfg LineConfig) LineConfig {
	if cfg.MaxLength <= 0 {
		cfg.MaxLength = 8192
	}
	return cfg
}

type byteBufEchoHandler struct{}

func (byteBufEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}
