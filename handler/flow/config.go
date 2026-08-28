package flow

const (
	defaultMaxPendingMessages = 1024
	defaultMaxPendingBytes    = 64 << 20
)

// Config 描述入站流控队列的硬保护边界。
type Config struct {
	// StartPaused 控制 Handler 加入 Pipeline 后是否立即暂停入站传播。
	StartPaused bool
	// MaxPendingMessages 是暂停期间最多保留的入站消息数，0 使用默认值。
	MaxPendingMessages int
	// MaxPendingBytes 是暂停期间最多保留的入站字节数，0 使用默认值。
	MaxPendingBytes int64
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.MaxPendingMessages < 0 || cfg.MaxPendingBytes < 0 {
		return Config{}, ErrInvalidConfig
	}
	if cfg.MaxPendingMessages == 0 {
		cfg.MaxPendingMessages = defaultMaxPendingMessages
	}
	if cfg.MaxPendingBytes == 0 {
		cfg.MaxPendingBytes = defaultMaxPendingBytes
	}
	return cfg, nil
}
