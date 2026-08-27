package recipes

import (
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

const (
	// HandlerNameHTTP1RequestDecoder 是 HTTP/1 服务端请求解码器的默认名称。
	HandlerNameHTTP1RequestDecoder = "http1-request-decoder"
	// HandlerNameHTTP1ResponseDecoder 是 HTTP/1 客户端响应解码器的默认名称。
	HandlerNameHTTP1ResponseDecoder = "http1-response-decoder"
	// HandlerNameHTTP1RequestEncoder 是 HTTP/1 客户端请求编码器的默认名称。
	HandlerNameHTTP1RequestEncoder = "http1-request-encoder"
	// HandlerNameHTTP1ResponseEncoder 是 HTTP/1 服务端响应编码器的默认名称。
	HandlerNameHTTP1ResponseEncoder = "http1-response-encoder"
	// HandlerNameHTTP1Continue 是 HTTP/1 100-continue 处理器的默认名称。
	HandlerNameHTTP1Continue = "http1-continue"
	// HandlerNameHTTP2FrameDecoder 是 HTTP/2 frame 解码器的默认名称。
	HandlerNameHTTP2FrameDecoder = "http2-frame-decoder"
	// HandlerNameHTTP2TypedDecoder 是 HTTP/2 typed frame 解码器的默认名称。
	HandlerNameHTTP2TypedDecoder = "http2-typed-decoder"
	// HandlerNameHTTP2FrameEncoder 是 HTTP/2 frame 编码器的默认名称。
	HandlerNameHTTP2FrameEncoder = "http2-frame-encoder"
	// HandlerNameHTTP2TypedEncoder 是 HTTP/2 typed frame 编码器的默认名称。
	HandlerNameHTTP2TypedEncoder = "http2-typed-encoder"
	// HandlerNameHTTP2Multiplexer 是 HTTP/2 stream multiplexer 的默认名称。
	HandlerNameHTTP2Multiplexer = "http2-multiplexer"
)

// HTTP1Config 描述 HTTP/1 pipeline 的帧边界。
type HTTP1Config struct {
	MaxHeaderBytes int
	MaxBodyBytes   int
}

// HTTP2Config 描述 HTTP/2 connection pipeline。
type HTTP2Config struct {
	MaxFrameSize int
	Multiplexer  http2.MultiplexerConfig
}

// HTTP1Server 创建 HTTP/1 服务端 pipeline。
func HTTP1Server(cfg HTTP1Config, app ...HandlerSpec) bootstrap.ChildInitializer {
	cfg = normalizeHTTP1Config(cfg)
	base := []HandlerSpec{
		UseFactory(HandlerNameHTTP1RequestDecoder, func() (channel.Handler, error) {
			return http1.NewRequestDecoder(cfg.MaxHeaderBytes, cfg.MaxBodyBytes)
		}),
		UseFactory(HandlerNameHTTP1ResponseEncoder, func() (channel.Handler, error) {
			return http1.NewResponseEncoder(), nil
		}),
		UseFactory(HandlerNameHTTP1Continue, func() (channel.Handler, error) {
			return http1.NewContinueHandler(), nil
		}),
	}
	return Initializer(appendSpecs(base, app)...)
}

// HTTP1Client 创建 HTTP/1 客户端 pipeline。
func HTTP1Client(cfg HTTP1Config, app ...HandlerSpec) bootstrap.ChildInitializer {
	cfg = normalizeHTTP1Config(cfg)
	base := []HandlerSpec{
		UseFactory(HandlerNameHTTP1ResponseDecoder, func() (channel.Handler, error) {
			return http1.NewResponseDecoder(cfg.MaxHeaderBytes, cfg.MaxBodyBytes)
		}),
		UseFactory(HandlerNameHTTP1RequestEncoder, func() (channel.Handler, error) {
			return http1.NewRequestEncoder(), nil
		}),
	}
	return Initializer(appendSpecs(base, app)...)
}

// HTTP2Connection 创建 HTTP/2 connection-level frame 和 stream pipeline。
func HTTP2Connection(cfg HTTP2Config, app ...HandlerSpec) bootstrap.ChildInitializer {
	base := []HandlerSpec{
		UseFactory(HandlerNameHTTP2FrameDecoder, func() (channel.Handler, error) {
			return http2.NewFrameDecoder(cfg.MaxFrameSize)
		}),
		UseFactory(HandlerNameHTTP2TypedDecoder, func() (channel.Handler, error) {
			return http2.NewTypedFrameDecoder(), nil
		}),
		UseFactory(HandlerNameHTTP2FrameEncoder, func() (channel.Handler, error) {
			return http2.NewFrameEncoder(), nil
		}),
		UseFactory(HandlerNameHTTP2TypedEncoder, func() (channel.Handler, error) {
			return http2.NewTypedFrameEncoder(), nil
		}),
		UseFactory(HandlerNameHTTP2Multiplexer, func() (channel.Handler, error) {
			return http2.NewStreamMultiplexer(cfg.Multiplexer)
		}),
	}
	return Initializer(appendSpecs(base, app)...)
}

func normalizeHTTP1Config(cfg HTTP1Config) HTTP1Config {
	if cfg.MaxHeaderBytes <= 0 {
		cfg.MaxHeaderBytes = 64 * 1024
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 8 * 1024 * 1024
	}
	return cfg
}
