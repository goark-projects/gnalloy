package l2

import "goark.dev/gnalloy/transport"

const defaultReadBufferSize = 65535

// Config 描述 L2 transport 的接口、协议过滤和驱动配置。
type Config struct {
	// InterfaceName 是需要绑定的网卡名称，优先级高于 InterfaceIndex。
	InterfaceName string
	// InterfaceIndex 是需要绑定的网卡索引，InterfaceName 为空时生效。
	InterfaceIndex int
	// EtherType 限制 native driver 接收的以太网协议类型，0 表示接收全部协议。
	EtherType uint16
	// Promiscuous 表示 driver 可按平台能力启用混杂模式。
	Promiscuous bool
	// ReadBufferSize 是单帧读取缓冲区大小，非正值使用默认值。
	ReadBufferSize int
	// WriteBufferWatermark 控制 Channel 出站水位线。
	WriteBufferWatermark transport.WriteBufferWatermark
	// Driver 是可注入的平台驱动；为空时使用当前平台 native driver。
	Driver Driver
}

// DefaultConfig 返回 L2 transport 的默认配置。
func DefaultConfig() Config {
	return Config{ReadBufferSize: defaultReadBufferSize}
}

func normalizeConfig(cfg Config, address string) Config {
	if cfg.InterfaceName == "" {
		cfg.InterfaceName = address
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = defaultReadBufferSize
	}
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	return cfg
}
