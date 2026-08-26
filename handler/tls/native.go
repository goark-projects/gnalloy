package tls

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

// EvaluateNativeProvider 根据能力矩阵判断 native TLS provider 是否满足 gnalloy 热路径要求。
func EvaluateNativeProvider(provider NativeProvider) NativeEvaluation {
	if provider == nil {
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

// UnsupportedNativeProvider 是默认的显式不可用 provider，占位而不引入 cgo/native 依赖。
type UnsupportedNativeProvider struct{}

// Capabilities 返回默认不可用能力集。
func (UnsupportedNativeProvider) Capabilities() NativeCapabilities {
	return NativeCapabilities{}
}
