package spdy

import "errors"

var (
	ErrInvalidFrame        = errors.New("spdy: invalid frame")
	ErrUnsupportedVersion  = errors.New("spdy: unsupported version")
	ErrUnsupportedFrame    = errors.New("spdy: unsupported frame")
	ErrTooManySettings     = errors.New("spdy: too many settings")
	ErrInvalidHeaderLength = errors.New("spdy: invalid header length")
)
