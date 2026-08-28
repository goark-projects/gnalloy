package deflate

import "compress/flate"

const DefaultMaxMessageBytes = 32 << 20

// Config 描述 permessage-deflate 的压缩级别和内存预算。
type Config struct {
	// CompressionLevel 为 0 时使用 flate.DefaultCompression。
	CompressionLevel int
	MaxMessageBytes  int
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.CompressionLevel == 0 {
		cfg.CompressionLevel = flate.DefaultCompression
	}
	if !validLevel(cfg.CompressionLevel) || cfg.MaxMessageBytes < 0 {
		return Config{}, ErrInvalidConfig
	}
	if cfg.MaxMessageBytes == 0 {
		cfg.MaxMessageBytes = DefaultMaxMessageBytes
	}
	return cfg, nil
}

func validLevel(level int) bool {
	return level == flate.DefaultCompression ||
		level == flate.HuffmanOnly ||
		(level >= flate.NoCompression && level <= flate.BestCompression)
}
