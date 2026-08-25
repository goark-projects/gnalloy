package quic

import "goark.dev/gnalloy/transport/udp"

const (
	MinInitialDatagramSize = 1200
	DefaultMaxDatagramSize = 1350
	DefaultIdleTimeoutMs   = 30000
)

type Config struct {
	Versions []Version

	MaxDatagramSize         int
	ActiveConnectionIDLimit int
	IdleTimeoutMillis       int64

	UDP udp.Config
}

func DefaultConfig() Config {
	return Config{
		Versions:                []Version{Version1},
		MaxDatagramSize:         DefaultMaxDatagramSize,
		ActiveConnectionIDLimit: 2,
		IdleTimeoutMillis:       DefaultIdleTimeoutMs,
		UDP:                     udp.DefaultConfig(),
	}
}

func NormalizeConfig(cfg Config) (Config, error) {
	def := DefaultConfig()
	if len(cfg.Versions) == 0 {
		cfg.Versions = def.Versions
	} else {
		versions := make([]Version, len(cfg.Versions))
		copy(versions, cfg.Versions)
		cfg.Versions = versions
	}
	for _, version := range cfg.Versions {
		if !version.Valid() {
			return Config{}, ErrInvalidVersion
		}
	}
	if cfg.MaxDatagramSize == 0 {
		cfg.MaxDatagramSize = def.MaxDatagramSize
	}
	if cfg.MaxDatagramSize < MinInitialDatagramSize {
		return Config{}, ErrInvalidConfig
	}
	if cfg.ActiveConnectionIDLimit == 0 {
		cfg.ActiveConnectionIDLimit = def.ActiveConnectionIDLimit
	}
	if cfg.ActiveConnectionIDLimit < 2 {
		return Config{}, ErrInvalidConfig
	}
	if cfg.IdleTimeoutMillis == 0 {
		cfg.IdleTimeoutMillis = def.IdleTimeoutMillis
	}
	if cfg.IdleTimeoutMillis < 0 {
		return Config{}, ErrInvalidConfig
	}
	if cfg.UDP.ReadBufferSize == 0 {
		cfg.UDP = def.UDP
	}
	return cfg, nil
}
