package lzf

import base "goark.dev/gnalloy/codec/compression"

// Config 描述 LZF 编解码边界的可调参数。
type Config struct {
	CompressThreshold int
	MaxDecodedBytes   int
}

func (c Config) encoderThreshold() (int, error) {
	threshold := c.CompressThreshold
	if threshold == 0 {
		threshold = minBlockToCompress
	}
	if threshold < minBlockToCompress || threshold > maxChunkLength {
		return 0, base.ErrInvalidConfig
	}
	return threshold, nil
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
