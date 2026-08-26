package rfc9000

import "errors"

var (
	// ErrMissingAddress 表示监听或拨号地址为空。
	ErrMissingAddress = errors.New("gnalloy/transport/quic/rfc9000: missing address")
	// ErrMissingTLSConfig 表示没有提供 QUIC 必需的 TLS 配置。
	ErrMissingTLSConfig = errors.New("gnalloy/transport/quic/rfc9000: missing tls config")
	// ErrInvalidTLSConfig 表示 TLS 配置与 QUIC/TLS 1.3 要求冲突。
	ErrInvalidTLSConfig = errors.New("gnalloy/transport/quic/rfc9000: invalid tls config")
	// ErrInvalidConfig 表示 QUIC 运行参数越界或互相冲突。
	ErrInvalidConfig = errors.New("gnalloy/transport/quic/rfc9000: invalid config")
	// ErrInvalidVersion 表示传入了当前适配层不支持的 QUIC 版本。
	ErrInvalidVersion = errors.New("gnalloy/transport/quic/rfc9000: invalid version")
	// Err0RTTDisabled 表示调用 0-RTT API 时没有显式启用 0-RTT。
	Err0RTTDisabled = errors.New("gnalloy/transport/quic/rfc9000: 0-rtt disabled")
	// ErrMissingSessionCache 表示客户端 0-RTT 缺少可复用的 TLS session cache。
	ErrMissingSessionCache = errors.New("gnalloy/transport/quic/rfc9000: missing session cache")
	// ErrUnsupportedWebTransport 表示当前 RFC9000 适配层尚未暴露 WebTransport 会话语义。
	ErrUnsupportedWebTransport = errors.New("gnalloy/transport/quic/rfc9000: unsupported webtransport")
	// ErrClosed 表示适配对象已经关闭或没有绑定底层实现。
	ErrClosed = errors.New("gnalloy/transport/quic/rfc9000: closed")
)
