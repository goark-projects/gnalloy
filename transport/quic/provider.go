package quic

import (
	quicprovider "goark.dev/gnalloy/transport/quic/provider"
	"goark.dev/gnalloy/transport/quic/provider/quicgo"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

type (
	// ProviderCapabilities 暴露 QUIC provider 的静态实现能力和配置能力评估。
	ProviderCapabilities = quicprovider.Capabilities
	// Provider 把 QUIC RFC9000 连接栈适配为可替换的连接工厂。
	Provider = quicprovider.Provider
	// ProviderEvaluation 是 provider 基础协议边界的静态评估结果。
	ProviderEvaluation = quicprovider.Evaluation
	// ProviderSnapshot 同时返回 provider 静态能力和客户端/服务端配置能力。
	ProviderSnapshot = quicprovider.Snapshot
	// QUICGoProvider 是默认 quic-go provider 的具体类型。
	QUICGoProvider = quicgo.Provider
)

// DetectNativeSupport 返回当前构建中 RFC9000 适配层的 provider 能力。
func DetectNativeSupport() NativeSupport {
	return rfc9000.DetectNativeSupport()
}

// EvaluateCapabilities 根据角色和配置返回 RFC9000 能力矩阵。
func EvaluateCapabilities(role EndpointRole, cfg Config) (CapabilitySet, error) {
	return rfc9000.EvaluateCapabilities(role, cfg)
}

// NewProvider 创建默认 quic-go provider。
func NewProvider(name ...string) QUICGoProvider {
	return quicgo.New(name...)
}

// DefaultProvider 返回默认 quic-go provider。
func DefaultProvider() QUICGoProvider {
	return quicgo.Default()
}

// EvaluateProvider 判断 provider 是否满足 RFC9000 QUIC v1 和 TLS 1.3 边界。
func EvaluateProvider(capabilities ProviderCapabilities) ProviderEvaluation {
	return quicprovider.Evaluate(capabilities)
}

// RequireProviderSupported 将 provider 能力评估转换为稳定错误，便于启动期 fail-fast。
func RequireProviderSupported(capabilities ProviderCapabilities) error {
	return quicprovider.RequireSupported(capabilities)
}

// InspectProvider 同时评估客户端和服务端配置，适合启动前输出能力快照。
func InspectProvider(capabilities ProviderCapabilities, clientCfg Config, serverCfg Config) (ProviderSnapshot, error) {
	return quicprovider.Inspect(capabilities, clientCfg, serverCfg)
}
