package rfc9000

// NativeProvider 描述 RFC9000 适配层当前委托的底层 QUIC 实现。
type NativeProvider string

const (
	// NativeProviderQUICGo 表示当前生产适配层使用 quic-go 协议栈。
	NativeProviderQUICGo NativeProvider = "quic-go"
)

// NativeSupport 是底层 QUIC provider 暴露给上层的稳定能力快照。
type NativeSupport struct {
	// Provider 是当前底层实现名称。
	Provider NativeProvider
	// RFC9000 表示 provider 支持 QUIC v1/RFC 9000。
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

// DetectNativeSupport 返回当前构建中 RFC9000 适配层的 provider 能力。
func DetectNativeSupport() NativeSupport {
	return NativeSupport{
		Provider:                   NativeProviderQUICGo,
		RFC9000:                    true,
		TLS13Only:                  true,
		ConnectionStats:            true,
		QLog:                       true,
		Datagrams:                  true,
		StreamResetPartialDelivery: true,
		ZeroRTT:                    true,
	}
}
