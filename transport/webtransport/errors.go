package webtransport

import "errors"

var (
	// ErrInvalidConnection 表示缺少有效 QUIC 连接。
	ErrInvalidConnection = errors.New("gnalloy/transport/webtransport: invalid connection")
	// ErrInvalidConnectStream 表示 CONNECT stream 不是有效 HTTP/3 request stream。
	ErrInvalidConnectStream = errors.New("gnalloy/transport/webtransport: invalid connect stream")
	// ErrInvalidSessionID 表示 CONNECT stream ID 不是客户端发起的双向 stream ID。
	ErrInvalidSessionID = errors.New("gnalloy/transport/webtransport: invalid session id")
	// ErrUnsupportedDatagram 表示 QUIC 连接未协商 RFC9221 datagram 能力。
	ErrUnsupportedDatagram = errors.New("gnalloy/transport/webtransport: datagram unsupported")
	// ErrUnsupportedStreamReset 表示 QUIC 连接未协商 WebTransport 依赖的 reset 扩展能力。
	ErrUnsupportedStreamReset = errors.New("gnalloy/transport/webtransport: stream reset partial delivery unsupported")
	// ErrInvalidStream 表示 WebTransport stream 无效或协议前缀非法。
	ErrInvalidStream = errors.New("gnalloy/transport/webtransport: invalid stream")
	// ErrReadUnsupported 表示当前 stream 不支持读取。
	ErrReadUnsupported = errors.New("gnalloy/transport/webtransport: stream read unsupported")
	// ErrWriteUnsupported 表示当前 stream 不支持写入。
	ErrWriteUnsupported = errors.New("gnalloy/transport/webtransport: stream write unsupported")
	// ErrClosed 表示 WebTransport stream 已关闭。
	ErrClosed = errors.New("gnalloy/transport/webtransport: stream closed")
	// ErrDatagramTooLarge 表示 datagram payload 超过配置上限。
	ErrDatagramTooLarge = errors.New("gnalloy/transport/webtransport: datagram too large")
	// ErrInvalidDatagram 表示 HTTP Datagram payload 缺少合法 Quarter Stream ID。
	ErrInvalidDatagram = errors.New("gnalloy/transport/webtransport: invalid datagram")
)
