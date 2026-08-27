package recipes

import (
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/websocket"
)

const (
	// HandlerNameWebSocketHandshake 是 WebSocket HTTP upgrade handler 的默认名称。
	HandlerNameWebSocketHandshake = "websocket-handshake"
	// HandlerNameWebSocketFrameDecoder 是 WebSocket frame 解码器的默认名称。
	HandlerNameWebSocketFrameDecoder = "websocket-frame-decoder"
	// HandlerNameWebSocketFrameEncoder 是 WebSocket frame 编码器的默认名称。
	HandlerNameWebSocketFrameEncoder = "websocket-frame-encoder"
	// HandlerNameWebSocketControl 是 WebSocket control frame handler 的默认名称。
	HandlerNameWebSocketControl = "websocket-control"
	// HandlerNameWebSocketUTF8 是 WebSocket UTF-8 校验器的默认名称。
	HandlerNameWebSocketUTF8 = "websocket-utf8"
	// HandlerNameWebSocketAggregator 是 WebSocket fragment 聚合器的默认名称。
	HandlerNameWebSocketAggregator = "websocket-aggregator"
)

// WebSocketServerConfig 描述 HTTP/1 upgrade 后的 WebSocket 服务端 pipeline。
type WebSocketServerConfig struct {
	Path               string
	HTTP               HTTP1Config
	MaxFrameLength     int
	ValidateUTF8       bool
	AggregateFragments bool
	MaxMessageLength   int
}

// WebSocketServer 创建 HTTP/1 + WebSocket 服务端 pipeline。
func WebSocketServer(cfg WebSocketServerConfig, app ...HandlerSpec) bootstrap.ChildInitializer {
	cfg = normalizeWebSocketServerConfig(cfg)
	removeOnUpgrade := []string{
		HandlerNameHTTP1RequestDecoder,
		HandlerNameHTTP1ResponseEncoder,
		HandlerNameHTTP1Continue,
		HandlerNameWebSocketHandshake,
	}
	base := []HandlerSpec{
		UseFactory(HandlerNameHTTP1RequestDecoder, func() (channel.Handler, error) {
			httpCfg := normalizeHTTP1Config(cfg.HTTP)
			return http1.NewRequestDecoder(httpCfg.MaxHeaderBytes, httpCfg.MaxBodyBytes)
		}),
		UseFactory(HandlerNameHTTP1ResponseEncoder, func() (channel.Handler, error) {
			return http1.NewResponseEncoder(), nil
		}),
		UseFactory(HandlerNameHTTP1Continue, func() (channel.Handler, error) {
			return http1.NewContinueHandler(), nil
		}),
		UseFactory(HandlerNameWebSocketHandshake, func() (channel.Handler, error) {
			return websocket.NewServerHandshakeHandler(cfg.Path, removeOnUpgrade...), nil
		}),
		UseFactory(HandlerNameWebSocketFrameDecoder, func() (channel.Handler, error) {
			return websocket.NewServerFrameDecoder(cfg.MaxFrameLength)
		}),
		UseFactory(HandlerNameWebSocketFrameEncoder, func() (channel.Handler, error) {
			return websocket.NewFrameEncoder(), nil
		}),
		UseFactory(HandlerNameWebSocketControl, func() (channel.Handler, error) {
			return websocket.NewControlFrameHandler(), nil
		}),
	}
	if cfg.ValidateUTF8 {
		base = append(base, UseFactory(HandlerNameWebSocketUTF8, func() (channel.Handler, error) {
			return websocket.NewUTF8Validator(), nil
		}))
	}
	if cfg.AggregateFragments {
		base = append(base, UseFactory(HandlerNameWebSocketAggregator, func() (channel.Handler, error) {
			return websocket.NewFragmentAggregator(cfg.MaxMessageLength), nil
		}))
	}
	return Initializer(appendSpecs(base, app)...)
}

func normalizeWebSocketServerConfig(cfg WebSocketServerConfig) WebSocketServerConfig {
	if cfg.MaxFrameLength <= 0 {
		cfg.MaxFrameLength = 1024 * 1024
	}
	if cfg.MaxMessageLength <= 0 {
		cfg.MaxMessageLength = cfg.MaxFrameLength
	}
	return cfg
}
