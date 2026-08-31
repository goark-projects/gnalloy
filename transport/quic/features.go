package quic

import (
	"crypto/tls"
	"fmt"
)

// EndpointRole 描述能力评估面向客户端还是服务端。
type EndpointRole uint8

const (
	// EndpointRoleClient 表示客户端 QUIC 连接侧。
	EndpointRoleClient EndpointRole = iota + 1
	// EndpointRoleServer 表示服务端 QUIC 监听侧。
	EndpointRoleServer
)

// FeatureCapability 描述单项能力的支持和启用状态。
type FeatureCapability struct {
	// Supported 表示适配层和底层 QUIC 栈具备该能力。
	Supported bool
	// Enabled 表示当前配置已开启该能力。
	Enabled bool
	// Reason 在能力不可用或未启用时给出稳定原因，便于上层诊断。
	Reason string
}

// CapabilitySet 描述 RFC9000 适配层在指定角色下的可用能力。
type CapabilitySet struct {
	// RFC9000 表示当前适配层以 QUIC v1/RFC9000 为生产协议边界。
	RFC9000 bool
	// TLS13 表示 TLS 边界满足 QUIC 必需的 TLS 1.3。
	TLS13 bool
	// SessionResumption 表示 TLS session resumption 能力。
	SessionResumption FeatureCapability
	// ZeroRTT 表示 QUIC 0-RTT 能力。
	ZeroRTT FeatureCapability
	// Datagrams 表示 RFC 9221 QUIC datagram 能力。
	Datagrams FeatureCapability
	// StreamResetPartialDelivery 表示带部分交付语义的 stream reset 能力。
	StreamResetPartialDelivery FeatureCapability
	// WebTransport 表示 WebTransport over HTTP/3 会话能力。
	WebTransport FeatureCapability
}

// EvaluateCapabilities 根据角色和配置返回可观测能力矩阵。
func EvaluateCapabilities(role EndpointRole, cfg Config) (CapabilitySet, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return CapabilitySet{}, err
	}
	if role != EndpointRoleClient && role != EndpointRoleServer {
		return CapabilitySet{}, fmt.Errorf("%w: invalid endpoint role", ErrInvalidConfig)
	}
	caps := CapabilitySet{
		RFC9000: true,
		TLS13:   normalized.tls.MinVersion >= tls.VersionTLS13,
		Datagrams: FeatureCapability{
			Supported: true,
			Enabled:   normalized.public.EnableDatagrams,
		},
		StreamResetPartialDelivery: FeatureCapability{
			Supported: true,
			Enabled:   normalized.public.EnableStreamResetPartialDelivery,
		},
		WebTransport: FeatureCapability{
			Supported: true,
			Enabled:   normalized.public.EnableWebTransport,
		},
	}
	if !caps.WebTransport.Enabled {
		caps.WebTransport.Reason = "配置未启用 WebTransport；HTTP/3 会话语义由 transport/webtransport 提供"
	}
	caps.SessionResumption = sessionResumptionCapability(role, normalized.public)
	caps.ZeroRTT = zeroRTTCapability(role, normalized.public, caps.SessionResumption)
	return caps, nil
}

func sessionResumptionCapability(role EndpointRole, cfg Config) FeatureCapability {
	capability := FeatureCapability{Supported: true}
	if cfg.TLS == nil {
		capability.Reason = "缺少 TLS 配置"
		return capability
	}
	if cfg.TLS.SessionTicketsDisabled {
		capability.Reason = "TLS session ticket 已关闭"
		return capability
	}
	if role == EndpointRoleClient && cfg.TLS.ClientSessionCache == nil {
		capability.Reason = "客户端缺少 TLS ClientSessionCache"
		return capability
	}
	capability.Enabled = true
	return capability
}

func zeroRTTCapability(role EndpointRole, cfg Config, session FeatureCapability) FeatureCapability {
	capability := FeatureCapability{Supported: true}
	if !cfg.Enable0RTT {
		capability.Reason = "配置未启用 0-RTT"
		return capability
	}
	if !session.Enabled {
		capability.Reason = session.Reason
		return capability
	}
	capability.Enabled = true
	return capability
}
