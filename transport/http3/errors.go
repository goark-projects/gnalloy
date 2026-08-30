package http3

import "errors"

var (
	ErrInvalidConnection = errors.New("gnalloy/transport/http3: invalid connection")
	ErrInvalidALPN       = errors.New("gnalloy/transport/http3: invalid alpn")
	ErrInvalidTLSState   = errors.New("gnalloy/transport/http3: invalid tls state")
	ErrInvalidStream     = errors.New("gnalloy/transport/http3: invalid stream")
	ErrReadUnsupported   = errors.New("gnalloy/transport/http3: stream read unsupported")
	ErrWriteUnsupported  = errors.New("gnalloy/transport/http3: stream write unsupported")
	ErrClosed            = errors.New("gnalloy/transport/http3: stream channel closed")
)
