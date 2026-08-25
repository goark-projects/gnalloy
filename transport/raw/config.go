package raw

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

const defaultReadBufferSize = 4096

// AllocatorFactory 为每个 raw endpoint 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

type Config struct {
	Protocol       int
	Family         Family
	HeaderIncluded bool
	ReadBufferSize int

	AllocatorFactory AllocatorFactory
}

func DefaultConfig() Config {
	return Config{Protocol: ProtocolICMP, Family: FamilyIPv4, ReadBufferSize: defaultReadBufferSize}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Protocol == 0 {
		cfg.Protocol = def.Protocol
	}
	if cfg.Family == 0 {
		cfg.Family = def.Family
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = def.ReadBufferSize
	}
	return cfg
}

type socketOptions struct {
	protocol       int
	family         Family
	headerIncluded bool
	readBufferSize int
}

func (c Config) socketOptions() (socketOptions, error) {
	if !validProtocol(c.Protocol) {
		return socketOptions{}, ErrInvalidProtocol
	}
	if c.Family != FamilyIPv4 && c.Family != FamilyIPv6 {
		return socketOptions{}, ErrInvalidAddress
	}
	return socketOptions{
		protocol:       c.Protocol,
		family:         c.Family,
		headerIncluded: c.HeaderIncluded,
		readBufferSize: c.ReadBufferSize,
	}, nil
}
