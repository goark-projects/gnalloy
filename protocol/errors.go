package protocol

import "errors"

var (
	// ErrInvalidConfig 表示协议装配缺少必要组件。
	ErrInvalidConfig = errors.New("gnalloy/protocol: invalid config")
	// ErrNoResponse 表示连接关闭前没有收到匹配响应。
	ErrNoResponse = errors.New("gnalloy/protocol: no matching response")
)
