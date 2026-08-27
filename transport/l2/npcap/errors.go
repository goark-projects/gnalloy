package npcap

import "errors"

var (
	// ErrMissingBackend 表示没有注入真实 Npcap 后端实现。
	ErrMissingBackend = errors.New("gnalloy/transport/l2/npcap: missing backend")
	// ErrInvalidConfig 表示 Npcap 边界配置无效。
	ErrInvalidConfig = errors.New("gnalloy/transport/l2/npcap: invalid config")
)
