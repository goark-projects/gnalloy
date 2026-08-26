package flush

const (
	// DefaultExplicitFlushAfterFlushes 是默认显式 flush 阈值，和 Netty 默认值保持一致。
	DefaultExplicitFlushAfterFlushes = 256
	// DefaultNoReadFlushDelayMillis 是无读循环聚合时的默认延迟。
	DefaultNoReadFlushDelayMillis int64 = 1
)

// Config 定义 flush 聚合策略。
type Config struct {
	// ExplicitFlushAfterFlushes 控制累计多少次 flush 后强制下发；零值使用默认值。
	ExplicitFlushAfterFlushes int
	// ConsolidateWhenNoReadInProgress 控制无读循环时是否通过定时器合并 flush。
	ConsolidateWhenNoReadInProgress bool
	// ConsolidateNoReadFlushDelayMillis 控制无读循环聚合的延迟；零值使用默认值。
	ConsolidateNoReadFlushDelayMillis int64
}

func normalizeConfig(config Config) (Config, error) {
	if config.ExplicitFlushAfterFlushes < 0 {
		return Config{}, ErrInvalidFlushThreshold
	}
	if config.ExplicitFlushAfterFlushes == 0 {
		config.ExplicitFlushAfterFlushes = DefaultExplicitFlushAfterFlushes
	}
	if config.ConsolidateNoReadFlushDelayMillis <= 0 {
		config.ConsolidateNoReadFlushDelayMillis = DefaultNoReadFlushDelayMillis
	}
	return config, nil
}
