package memcache

import "errors"

var (
	ErrInvalidFrame = errors.New("gnalloy/codec/memcache: invalid frame")
	// ErrInvalidConfig 表示 memcache 编解码器配置非法。
	ErrInvalidConfig = errors.New("gnalloy/codec/memcache: invalid config")
)
