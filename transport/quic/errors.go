package quic

import "goark.dev/gnalloy/transport/quic/rfc9000"

var (
	// ErrMissingAddress 表示监听或拨号地址为空。
	ErrMissingAddress = rfc9000.ErrMissingAddress
	// ErrMissingTLSConfig 表示没有提供 QUIC 必需的 TLS 配置。
	ErrMissingTLSConfig = rfc9000.ErrMissingTLSConfig
	// ErrInvalidTLSConfig 表示 TLS 配置与 QUIC/TLS 1.3 要求冲突。
	ErrInvalidTLSConfig = rfc9000.ErrInvalidTLSConfig
	// ErrInvalidConfig 表示 QUIC 运行参数越界或互相冲突。
	ErrInvalidConfig = rfc9000.ErrInvalidConfig
	// ErrInvalidVersion 表示传入了当前适配层不支持的 QUIC 版本。
	ErrInvalidVersion = rfc9000.ErrInvalidVersion
	// Err0RTTDisabled 表示调用 0-RTT API 时没有显式启用 0-RTT。
	Err0RTTDisabled = rfc9000.Err0RTTDisabled
	// ErrMissingSessionCache 表示客户端 0-RTT 缺少可复用的 TLS session cache。
	ErrMissingSessionCache = rfc9000.ErrMissingSessionCache
	// ErrUnsupportedWebTransport 表示底层能力不满足 WebTransport prerequisites。
	ErrUnsupportedWebTransport = rfc9000.ErrUnsupportedWebTransport
	// ErrClosed 表示适配对象已经关闭或没有绑定底层实现。
	ErrClosed = rfc9000.ErrClosed
	// ErrInvalidStream 表示缺少可读写的 QUIC stream。
	ErrInvalidStream = rfc9000.ErrInvalidStream
	// ErrWriteUnsupported 表示出站消息无法写入 QUIC stream。
	ErrWriteUnsupported = rfc9000.ErrWriteUnsupported
)
