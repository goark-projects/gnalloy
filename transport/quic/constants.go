package quic

import "goark.dev/gnalloy/transport/quic/rfc9000"

const (
	// DefaultALPN 是未显式指定应用协议时使用的 Gnalloy QUIC ALPN。
	DefaultALPN = rfc9000.DefaultALPN
	// MinInitialPacketSize 是 RFC 9000 要求 Initial 包承载的最小 UDP payload。
	MinInitialPacketSize = rfc9000.MinInitialPacketSize
	// DefaultClientTokenStoreMaxOrigins 是 0-RTT 地址验证 token store 的默认 origin 数。
	DefaultClientTokenStoreMaxOrigins = rfc9000.DefaultClientTokenStoreMaxOrigins
	// DefaultClientTokenStoreTokensPerOrigin 是每个 origin 保留的默认 token 数。
	DefaultClientTokenStoreTokensPerOrigin = rfc9000.DefaultClientTokenStoreTokensPerOrigin
)

const (
	// Version1 是 RFC 9000 定义的 QUIC v1。
	Version1 Version = rfc9000.Version1
)

const (
	// EndpointRoleClient 表示客户端 QUIC 连接侧。
	EndpointRoleClient EndpointRole = rfc9000.EndpointRoleClient
	// EndpointRoleServer 表示服务端 QUIC 监听侧。
	EndpointRoleServer EndpointRole = rfc9000.EndpointRoleServer
)

const (
	// NativeProviderQUICGo 表示当前生产适配层使用 quic-go 协议栈。
	NativeProviderQUICGo NativeProvider = rfc9000.NativeProviderQUICGo
)
