package quic

// NativeEngine 描述当前委托的底层 QUIC 协议引擎。
type NativeEngine string

const (
	// NativeEngineQUICGo 表示当前生产适配层使用 quic-go 协议栈。
	NativeEngineQUICGo NativeEngine = "quic-go"
)

// NativeSupport 是底层 QUIC 协议引擎暴露给上层的稳定能力快照。
type NativeSupport struct {
	// Engine 是当前底层协议引擎名称。
	Engine NativeEngine
	// RFC9000 表示协议引擎支持 QUIC v1/RFC 9000。
	RFC9000 bool
	// TLS13Only 表示 QUIC packet protection 固定在 TLS 1.3 语义。
	TLS13Only bool
	// ConnectionStats 表示连接级 RTT、包和字节计数可读取。
	ConnectionStats bool
	// QLog 表示可以为每条连接创建 qlog trace。
	QLog bool
	// Datagrams 表示支持 RFC 9221 QUIC datagram。
	Datagrams bool
	// StreamResetPartialDelivery 表示支持带部分交付语义的 stream reset。
	StreamResetPartialDelivery bool
	// ZeroRTT 表示支持 QUIC 0-RTT 配置边界。
	ZeroRTT bool
}

// DetectNativeSupport 返回当前构建中 QUIC 协议引擎的能力。
func DetectNativeSupport() NativeSupport {
	return NativeSupport{
		Engine:                     NativeEngineQUICGo,
		RFC9000:                    true,
		TLS13Only:                  true,
		ConnectionStats:            true,
		QLog:                       true,
		Datagrams:                  true,
		StreamResetPartialDelivery: true,
		ZeroRTT:                    true,
	}
}
