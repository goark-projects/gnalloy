package deflate

import "errors"

var (
	// ErrInvalidConfig 表示 permessage-deflate 配置非法。
	ErrInvalidConfig = errors.New("gnalloy/codec/websocket/deflate: invalid config")
	// ErrInvalidExtension 表示 Sec-WebSocket-Extensions 参数非法。
	ErrInvalidExtension = errors.New("gnalloy/codec/websocket/deflate: invalid extension")
	// ErrInvalidFrame 表示 frame 的 RSV、opcode 或分片状态不符合 permessage-deflate。
	ErrInvalidFrame = errors.New("gnalloy/codec/websocket/deflate: invalid frame")
)
