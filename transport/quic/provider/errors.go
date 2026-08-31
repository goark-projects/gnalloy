package provider

import "errors"

var (
	// ErrUnsupportedProvider 表示 QUIC provider 缺失或不满足基础协议边界。
	ErrUnsupportedProvider = errors.New("gnalloy/transport/quic/provider: unsupported provider")
)
