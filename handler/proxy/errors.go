package proxy

import "errors"

var (
	ErrNeedMore            = errors.New("gnalloy/handler/proxy: need more bytes")
	ErrInvalidMessage      = errors.New("gnalloy/handler/proxy: invalid message")
	ErrHandshakeFailed     = errors.New("gnalloy/handler/proxy: handshake failed")
	ErrUnsupportedAddress  = errors.New("gnalloy/handler/proxy: unsupported address")
	ErrUnsupportedProtocol = errors.New("gnalloy/handler/proxy: unsupported protocol")
)
