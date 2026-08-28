package zerocopy

import "errors"

var (
	// ErrUnsupported 表示当前平台或 FileRegion 不具备原生零拷贝条件。
	ErrUnsupported = errors.New("gnalloy/transport/zerocopy: unsupported zero-copy transfer")
	// ErrInvalidConfig 表示 Sender 配置非法。
	ErrInvalidConfig = errors.New("gnalloy/transport/zerocopy: invalid config")
)
