package bpf

import "errors"

var (
	// ErrMissingBackend 表示没有注入真实 BPF 后端实现。
	ErrMissingBackend = errors.New("gnalloy/transport/l2/bpf: missing backend")
	// ErrInvalidConfig 表示 BPF 边界配置无效。
	ErrInvalidConfig = errors.New("gnalloy/transport/l2/bpf: invalid config")
	// ErrUnavailable 表示 BPF 设备不可用、未授权或打开失败。
	ErrUnavailable = errors.New("gnalloy/transport/l2/bpf: unavailable")
)
