package protobuf

import "errors"

var (
	// ErrInvalidConfig 表示 protobuf 编解码器配置非法。
	ErrInvalidConfig = errors.New("gnalloy/codec/protobuf: invalid config")
)
