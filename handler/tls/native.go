package tls

import (
	cryptotls "crypto/tls"
	"fmt"
	"net"
	"reflect"
	"strings"
)

// Conn 是 TLS 引擎对 handler 暴露的最小连接接口。
type Conn interface {
	Handshake() error
	ConnectionState() cryptotls.ConnectionState
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

// Provider 描述可插拔 TLS 引擎的连接工厂边界。
type Provider interface {
	NativeProvider
	Client(conn net.Conn, cfg *cryptotls.Config) (Conn, error)
	Server(conn net.Conn, cfg *cryptotls.Config) (Conn, error)
}

// NativeCapabilities 描述可选 native TLS 引擎能提供的能力。
type NativeCapabilities struct {
	Provider             string
	TLS13                bool
	ALPN                 bool
	SNI                  bool
	QUICPacketProtection bool
	ZeroCopyRead         bool
	ZeroCopyWrite        bool
	RequiresCGO          bool
}

// NativeProvider 描述 native TLS 引擎的能力探测边界。
type NativeProvider interface {
	Capabilities() NativeCapabilities
}

// NativeEvaluation 是 native TLS 路线是否可接入的静态评估结果。
type NativeEvaluation struct {
	Supported bool
	Reasons   []string
}

// CryptoProvider 是默认 TLS provider，基于 Go 标准库 crypto/tls。
type CryptoProvider struct{}

// Capabilities 返回 crypto/tls 在 handler 场景下的能力快照。
func (CryptoProvider) Capabilities() NativeCapabilities {
	return NativeCapabilities{
		Provider: "crypto/tls",
		TLS13:    true,
		ALPN:     true,
		SNI:      true,
	}
}

// Client 创建客户端 TLS 连接。
func (CryptoProvider) Client(conn net.Conn, cfg *cryptotls.Config) (Conn, error) {
	return cryptotls.Client(conn, cfg), nil
}

// Server 创建服务端 TLS 连接。
func (CryptoProvider) Server(conn net.Conn, cfg *cryptotls.Config) (Conn, error) {
	return cryptotls.Server(conn, cfg), nil
}

// EvaluateNativeProvider 根据能力矩阵判断 native TLS provider 是否满足 gnalloy 热路径要求。
func EvaluateNativeProvider(provider NativeProvider) NativeEvaluation {
	if nativeProviderMissing(provider) {
		return NativeEvaluation{Reasons: []string{"未提供 native TLS provider"}}
	}
	capabilities := provider.Capabilities()
	reasons := make([]string, 0, 4)
	if capabilities.Provider == "" {
		reasons = append(reasons, "provider 名称为空")
	}
	if !capabilities.TLS13 {
		reasons = append(reasons, "缺少 TLS 1.3")
	}
	if !capabilities.ALPN {
		reasons = append(reasons, "缺少 ALPN")
	}
	if !capabilities.QUICPacketProtection {
		reasons = append(reasons, "缺少 QUIC packet protection")
	}
	return NativeEvaluation{Supported: len(reasons) == 0, Reasons: reasons}
}

// EvaluateProvider 判断 TLS handler 可插拔 provider 是否满足连接级 TLS 能力。
func EvaluateProvider(provider Provider) NativeEvaluation {
	if nativeProviderMissing(provider) {
		return NativeEvaluation{Reasons: []string{"未提供 TLS provider"}}
	}
	capabilities := provider.Capabilities()
	reasons := make([]string, 0, 3)
	if capabilities.Provider == "" {
		reasons = append(reasons, "provider 名称为空")
	}
	if !capabilities.TLS13 {
		reasons = append(reasons, "缺少 TLS 1.3")
	}
	if !capabilities.ALPN {
		reasons = append(reasons, "缺少 ALPN")
	}
	if !capabilities.SNI {
		reasons = append(reasons, "缺少 SNI")
	}
	return NativeEvaluation{Supported: len(reasons) == 0, Reasons: reasons}
}

func normalizeProvider(provider Provider) Provider {
	if nativeProviderMissing(provider) {
		return CryptoProvider{}
	}
	return provider
}

func nativeProviderMissing(provider NativeProvider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func providerEvaluationError(evaluation NativeEvaluation) error {
	if evaluation.Supported {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNativeTLSUnavailable, strings.Join(evaluation.Reasons, "; "))
}

// UnsupportedNativeProvider 是默认的显式不可用 provider，占位而不引入 cgo/native 依赖。
type UnsupportedNativeProvider struct{}

// Capabilities 返回默认不可用能力集。
func (UnsupportedNativeProvider) Capabilities() NativeCapabilities {
	return NativeCapabilities{}
}
