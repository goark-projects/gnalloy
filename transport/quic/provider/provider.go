package provider

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"goark.dev/gnalloy/transport/quic/rfc9000"
)

// Capabilities 暴露 QUIC provider 的静态实现能力和配置能力评估。
type Capabilities interface {
	NativeSupport() rfc9000.NativeSupport
	EvaluateCapabilities(role rfc9000.EndpointRole, cfg rfc9000.Config) (rfc9000.CapabilitySet, error)
}

// Provider 把 QUIC RFC9000 连接栈适配为可替换的连接工厂。
type Provider interface {
	Capabilities
	ListenAddr(addr string, cfg rfc9000.Config) (rfc9000.Listener, error)
	ListenAddrEarly(addr string, cfg rfc9000.Config) (rfc9000.EarlyListener, error)
	DialAddr(ctx context.Context, addr string, cfg rfc9000.Config) (rfc9000.Connection, error)
	DialAddrEarly(ctx context.Context, addr string, cfg rfc9000.Config) (rfc9000.Connection, error)
}

// Evaluation 是 provider 基础协议边界的静态评估结果。
type Evaluation struct {
	Supported bool
	Native    rfc9000.NativeSupport
	Reasons   []string
}

// Snapshot 同时返回 provider 静态能力和客户端/服务端配置能力。
type Snapshot struct {
	Native rfc9000.NativeSupport
	Client rfc9000.CapabilitySet
	Server rfc9000.CapabilitySet
}

// Evaluate 判断 provider 是否满足 RFC9000 QUIC v1 和 TLS 1.3 packet protection 边界。
func Evaluate(capabilities Capabilities) Evaluation {
	if providerMissing(capabilities) {
		return Evaluation{Reasons: []string{"未提供 QUIC provider"}}
	}
	native := capabilities.NativeSupport()
	reasons := make([]string, 0, 3)
	if native.Provider == "" {
		reasons = append(reasons, "provider 名称为空")
	}
	if !native.RFC9000 {
		reasons = append(reasons, "缺少 RFC9000 QUIC v1")
	}
	if !native.TLS13Only {
		reasons = append(reasons, "缺少 QUIC TLS 1.3 packet protection 边界")
	}
	return Evaluation{
		Supported: len(reasons) == 0,
		Native:    native,
		Reasons:   reasons,
	}
}

// RequireSupported 将 provider 能力评估转换为稳定错误，便于启动期 fail-fast。
func RequireSupported(capabilities Capabilities) error {
	evaluation := Evaluate(capabilities)
	if evaluation.Supported {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedProvider, strings.Join(evaluation.Reasons, "; "))
}

// Inspect 同时评估客户端和服务端配置，适合启动前输出能力快照。
func Inspect(capabilities Capabilities, clientCfg rfc9000.Config, serverCfg rfc9000.Config) (Snapshot, error) {
	if err := RequireSupported(capabilities); err != nil {
		return Snapshot{}, err
	}
	client, err := capabilities.EvaluateCapabilities(rfc9000.EndpointRoleClient, clientCfg)
	if err != nil {
		return Snapshot{}, err
	}
	server, err := capabilities.EvaluateCapabilities(rfc9000.EndpointRoleServer, serverCfg)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Native: capabilities.NativeSupport(),
		Client: client,
		Server: server,
	}, nil
}

func providerMissing(capabilities Capabilities) bool {
	if capabilities == nil {
		return true
	}
	value := reflect.ValueOf(capabilities)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
