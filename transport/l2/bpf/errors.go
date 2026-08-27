package bpf

import "errors"

var (
	// ErrMissingBackend 表示没有注入真实 BPF 后端实现。
	ErrMissingBackend = errors.New("gnalloy/transport/l2/bpf: missing backend")
	// ErrInvalidConfig 表示 BPF 边界配置无效。
	ErrInvalidConfig = errors.New("gnalloy/transport/l2/bpf: invalid config")
)
