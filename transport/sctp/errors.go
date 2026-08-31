package sctp

import "errors"

var (
	ErrInvalidAddress        = errors.New("gnalloy/transport/sctp: invalid sctp address")
	ErrConnectTimeout        = errors.New("gnalloy/transport/sctp: connect timeout")
	ErrServerClosed          = errors.New("gnalloy/transport/sctp: server closed")
	ErrCloseActiveTimeout    = errors.New("gnalloy/transport/sctp: timeout closing active child channel")
	ErrUnsupportedSCTP       = errors.New("gnalloy/transport/sctp: sctp transport is unsupported on this platform")
	ErrUnsupportedCompletion = errors.New("gnalloy/transport/sctp: completion poller is unsupported for sctp")
)
