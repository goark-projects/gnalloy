package raw

import "errors"

var (
	ErrInvalidAddress        = errors.New("gnalloy/transport/raw: invalid address")
	ErrInvalidPacket         = errors.New("gnalloy/transport/raw: invalid packet")
	ErrInvalidProtocol       = errors.New("gnalloy/transport/raw: invalid protocol")
	ErrServerClosed          = errors.New("gnalloy/transport/raw: server closed")
	ErrUnsupportedCompletion = errors.New("gnalloy/transport/raw: selected completion backend does not support packet ops")
)
