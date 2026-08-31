package fastlz

import base "goark.dev/gnalloy/codec/compression"

// Level 描述 FastLZ 压缩级别。
type Level uint8

const (
	LevelAuto Level = iota
	Level1
	Level2
)

// Config 描述 FastLZ frame 的编解码参数。
type Config struct {
	Level           Level
	Checksum        bool
	MaxDecodedBytes int
}

func (c Config) encoderLevel() (Level, error) {
	switch c.Level {
	case LevelAuto, Level1, Level2:
		return c.Level, nil
	default:
		return 0, base.ErrInvalidConfig
	}
}

func (c Config) decoderLimit() (int, error) {
	limit := c.MaxDecodedBytes
	if limit < 0 {
		return 0, base.ErrInvalidConfig
	}
	if limit == 0 {
		limit = base.DefaultMaxDecodedBytes
	}
	return limit, nil
}
