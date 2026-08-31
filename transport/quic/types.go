package quic

import "goark.dev/gnalloy/transport/quic/rfc9000"

type (
	// Version 是 Gnalloy QUIC 门面公开的版本号类型。
	Version = rfc9000.Version
	// Listener 是 RFC9000 QUIC 服务端监听器接口。
	Listener = rfc9000.Listener
	// EarlyListener 是可在握手完成前接受 0-RTT 连接的监听器接口。
	EarlyListener = rfc9000.EarlyListener
	// Connection 是 RFC9000 QUIC 连接接口。
	Connection = rfc9000.Connection
	// Stream 是 QUIC 双向 stream；Close 只关闭发送方向并发送 FIN。
	Stream = rfc9000.Stream
	// SendStream 是 QUIC 单向发送 stream。
	SendStream = rfc9000.SendStream
	// ReceiveStream 是 QUIC 单向接收 stream。
	ReceiveStream = rfc9000.ReceiveStream
	// StreamID 是 QUIC stream 的稳定公开标识。
	StreamID = rfc9000.StreamID
	// ApplicationErrorCode 是 QUIC 应用层连接关闭错误码。
	ApplicationErrorCode = rfc9000.ApplicationErrorCode
	// StreamErrorCode 是 QUIC stream reset 或 stop-sending 错误码。
	StreamErrorCode = rfc9000.StreamErrorCode
	// FeatureSupport 描述本端和对端是否都支持某项 QUIC 扩展能力。
	FeatureSupport = rfc9000.FeatureSupport
	// State 是不暴露底层实现类型的 QUIC 连接状态快照。
	State = rfc9000.State
	// ConnectionStats 是不暴露底层实现类型的 QUIC 连接统计快照。
	ConnectionStats = rfc9000.ConnectionStats
	// Dialer 抽象 RFC9000 QUIC 客户端拨号能力。
	Dialer = rfc9000.Dialer
	// EarlyDialer 抽象 RFC9000 QUIC 0-RTT 客户端拨号能力。
	EarlyDialer = rfc9000.EarlyDialer
	// DialerFunc 允许普通函数作为 Dialer 使用。
	DialerFunc = rfc9000.DialerFunc
	// DefaultDialer 是使用系统 UDP socket 的默认 QUIC 拨号器。
	DefaultDialer = rfc9000.DefaultDialer
	// EndpointRole 描述能力评估面向客户端还是服务端。
	EndpointRole = rfc9000.EndpointRole
	// FeatureCapability 描述单项能力的支持和启用状态。
	FeatureCapability = rfc9000.FeatureCapability
	// CapabilitySet 描述 RFC9000 适配层在指定角色下的可用能力。
	CapabilitySet = rfc9000.CapabilitySet
	// NativeProvider 描述 RFC9000 适配层当前委托的底层 QUIC 实现。
	NativeProvider = rfc9000.NativeProvider
	// NativeSupport 是底层 QUIC provider 暴露给上层的稳定能力快照。
	NativeSupport = rfc9000.NativeSupport
	// ClientToken 是客户端收到的 QUIC NEW_TOKEN。
	ClientToken = rfc9000.ClientToken
	// ClientTokenStore 保存客户端 NEW_TOKEN，供后续连接跳过地址验证。
	ClientTokenStore = rfc9000.ClientTokenStore
	// QLogTraceInfo 描述即将创建的 qlog trace。
	QLogTraceInfo = rfc9000.QLogTraceInfo
	// QLogWriterFactory 为每条连接打开独立 qlog 输出。
	QLogWriterFactory = rfc9000.QLogWriterFactory
	// QLogWriterFactoryFunc 允许函数直接作为 qlog writer factory。
	QLogWriterFactoryFunc = rfc9000.QLogWriterFactoryFunc
	// QLogConfig 描述 RFC 9000 适配层的 qlog 输出边界。
	QLogConfig = rfc9000.QLogConfig
)
