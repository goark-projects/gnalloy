package tcp

import "errors"

var (
	ErrInvalidAddress              = errors.New("gnalloy/transport/tcp: invalid tcp address")
	ErrConnectTimeout              = errors.New("gnalloy/transport/tcp: connect timeout")
	ErrCloseActiveTimeout          = errors.New("gnalloy/transport/tcp: timeout closing active child channel")
	ErrServerClosed                = errors.New("gnalloy/transport/tcp: server closed")
	ErrUnsupportedTCP              = errors.New("gnalloy/transport/tcp: tcp transport is unsupported on this platform")
	ErrUnsupportedCompletionAccept = errors.New("gnalloy/transport/tcp: completion accept is unsupported on this platform")
	ErrUnsupportedFixedBuffers     = errors.New("gnalloy/transport/tcp: io_uring fixed buffers are unsupported for this allocator or backend")
	ErrUnsupportedReusePort        = errors.New("gnalloy/transport/tcp: SO_REUSEPORT is unsupported on this platform")
)
