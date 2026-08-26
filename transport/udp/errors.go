package udp

import "errors"

var (
	ErrInvalidAddress        = errors.New("gnalloy/transport/udp: invalid address")
	ErrInvalidDatagram       = errors.New("gnalloy/transport/udp: invalid datagram")
	ErrServerClosed          = errors.New("gnalloy/transport/udp: server closed")
	ErrUnsupportedReusePort  = errors.New("gnalloy/transport/udp: SO_REUSEPORT is unsupported")
	ErrUnsupportedCompletion = errors.New("gnalloy/transport/udp: selected completion backend does not support datagram ops")
)
