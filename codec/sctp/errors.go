package sctp

import "errors"

var (
	ErrInvalidMessage       = errors.New("gnalloy/codec/sctp: invalid sctp message")
	ErrMissingFragmentStart = errors.New("gnalloy/codec/sctp: missing fragment start")
)
