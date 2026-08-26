package application

import "errors"

var (
	// ErrFrameTooLarge 表示应用帧超过本地上限。
	ErrFrameTooLarge = errors.New("gnalloy/transport/quic/application: frame too large")
	// ErrInvalidConfig 表示应用协议装配缺少必要配置。
	ErrInvalidConfig = errors.New("gnalloy/transport/quic/application: invalid config")
)
