package udt

import "goark.dev/gnalloy/transport"

const defaultReadBufferSize = 64 * 1024

type Config struct {
	ReadBufferSize       int
	WriteBufferWatermark transport.WriteBufferWatermark
	Driver               Driver
}

func DefaultConfig() Config {
	return Config{
		ReadBufferSize:       defaultReadBufferSize,
		WriteBufferWatermark: transport.DefaultWriteBufferWatermark(),
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = defaultReadBufferSize
	}
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	return cfg
}
