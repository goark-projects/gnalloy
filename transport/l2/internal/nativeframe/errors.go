package nativeframe

import "errors"

var (
	// ErrInvalidConfig 表示平台原生 L2 driver 配置无效。
	ErrInvalidConfig = errors.New("gnalloy/transport/l2/internal/nativeframe: invalid config")
	// ErrUnsupportedDriver 表示当前平台没有对应原生 driver。
	ErrUnsupportedDriver = errors.New("gnalloy/transport/l2/internal/nativeframe: unsupported driver")
	// ErrUnavailable 表示平台驱动或运行时库未安装、未授权或不可打开。
	ErrUnavailable = errors.New("gnalloy/transport/l2/internal/nativeframe: driver unavailable")
	// ErrInvalidFrame 表示底层驱动返回了不完整或不可发送的二层帧。
	ErrInvalidFrame = errors.New("gnalloy/transport/l2/internal/nativeframe: invalid frame")
	// ErrClosed 表示 endpoint 已关闭。
	ErrClosed = errors.New("gnalloy/transport/l2/internal/nativeframe: endpoint closed")
)
