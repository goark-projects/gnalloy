package bpf

import (
	"fmt"
	"time"

	"goark.dev/gnalloy/transport/l2"
)

const defaultSnapshotLength = 65535

// Config 描述 BPF 后端打开二层接口所需的平台无关参数。
type Config struct {
	// InterfaceName 是 BPF 绑定的网卡名称。
	InterfaceName string
	// InterfaceIndex 是操作系统分配的网卡索引，InterfaceName 为空时供后端解析。
	InterfaceIndex int
	// EtherType 是可选以太网协议过滤条件，0 表示不过滤。
	EtherType uint16
	// Promiscuous 表示后端可按平台能力启用混杂模式。
	Promiscuous bool
	// SnapshotLength 是单帧截断长度，0 使用默认完整以太网帧长度。
	SnapshotLength int
	// Immediate 表示后端应优先使用低延迟立即模式。
	Immediate bool
	// ReadTimeout 是 BPF read 超时，0 表示由后端使用阻塞读取或平台默认值。
	ReadTimeout time.Duration
}

func normalizeConfig(base Config, l2cfg l2.Config) (Config, error) {
	out := base
	if l2cfg.InterfaceName != "" {
		out.InterfaceName = l2cfg.InterfaceName
	}
	if l2cfg.InterfaceIndex > 0 {
		out.InterfaceIndex = l2cfg.InterfaceIndex
	}
	if l2cfg.EtherType != 0 {
		out.EtherType = l2cfg.EtherType
	}
	if l2cfg.Promiscuous {
		out.Promiscuous = true
	}
	if out.SnapshotLength == 0 {
		out.SnapshotLength = defaultSnapshotLength
	}
	if out.SnapshotLength < 0 {
		return Config{}, fmt.Errorf("%w: %w", l2.ErrInvalidConfig, ErrInvalidConfig)
	}
	if out.ReadTimeout < 0 {
		return Config{}, fmt.Errorf("%w: %w", l2.ErrInvalidConfig, ErrInvalidConfig)
	}
	if out.InterfaceName == "" && out.InterfaceIndex <= 0 {
		return Config{}, fmt.Errorf("%w: %w", l2.ErrInvalidConfig, ErrInvalidConfig)
	}
	return out, nil
}
