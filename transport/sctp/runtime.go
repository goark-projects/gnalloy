package sctp

import (
	"fmt"
	"runtime"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/transport"
)

// EndpointRole 描述 SCTP runtime 校验面向服务端还是客户端。
type EndpointRole uint8

const (
	// EndpointRoleClient 表示 SCTP 客户端 Dialer。
	EndpointRoleClient EndpointRole = iota + 1
	// EndpointRoleServer 表示 SCTP 服务端 ServerBootstrap。
	EndpointRoleServer
)

// RuntimeSupport 描述当前构建能提供的 SCTP transport 能力边界。
type RuntimeSupport struct {
	Platform         string
	NativeSocket     bool
	ReadinessPoller  bool
	CompletionPoller bool
	OneToOneStream   bool
}

// RuntimeCheck 是 SCTP 启动前 runtime 校验的输入快照。
type RuntimeCheck struct {
	Role        EndpointRole
	Address     string
	Config      Config
	Group       *transport.EventLoopGroup
	BossGroup   *transport.EventLoopGroup
	WorkerGroup *transport.EventLoopGroup
}

// ValidateConfig 校验 SCTP 配置中不能被归一化安全修复的边界。
func ValidateConfig(cfg Config) error {
	if cfg.SendBufferSize < 0 {
		return fmt.Errorf("%w: negative send buffer size", ErrInvalidConfig)
	}
	if cfg.ReceiveBufferSize < 0 {
		return fmt.Errorf("%w: negative receive buffer size", ErrInvalidConfig)
	}
	return nil
}

// ValidateRuntime 在打开 SCTP fd 前校验平台、地址和 EventLoop 后端边界。
func ValidateRuntime(check RuntimeCheck) error {
	if err := ValidateConfig(check.Config); err != nil {
		return err
	}
	address, err := parseAddress(check.Address)
	if err != nil {
		return err
	}
	switch check.Role {
	case EndpointRoleClient:
		if address.port == 0 {
			return ErrInvalidAddress
		}
		if err := validateReadinessGroup(check.Group); err != nil {
			return err
		}
	case EndpointRoleServer:
		if err := validateReadinessGroup(check.BossGroup); err != nil {
			return err
		}
		if err := validateReadinessGroup(check.WorkerGroup); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: invalid endpoint role", ErrInvalidConfig)
	}
	if !DetectRuntimeSupport().NativeSocket {
		return ErrUnsupportedSCTP
	}
	return nil
}

func validateReadinessGroup(group *transport.EventLoopGroup) error {
	if group == nil {
		return bootstrap.ErrMissingGroup
	}
	loops := group.Loops()
	if len(loops) == 0 {
		return transport.ErrNoEventLoop
	}
	for _, loop := range loops {
		if loop == nil || loop.Poller() == nil {
			return transport.ErrNoEventLoop
		}
		if loop.Poller().Model() == transport.PollerCompletion {
			return ErrUnsupportedCompletion
		}
	}
	return nil
}

// DetectRuntimeSupport 返回当前构建静态 SCTP 能力快照。
func DetectRuntimeSupport() RuntimeSupport {
	return detectRuntimeSupport()
}

func runtimePlatform() string {
	return runtime.GOOS
}
