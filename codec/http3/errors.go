package http3

import "errors"

var (
	ErrInvalidFrame       = errors.New("http3: invalid frame")
	ErrFrameTooLarge      = errors.New("http3: frame too large")
	ErrInvalidVarInt      = errors.New("http3: invalid varint")
	ErrDuplicateSetting   = errors.New("http3: duplicate setting")
	ErrTooManySettings    = errors.New("http3: too many settings")
	ErrUnsupportedFrame   = errors.New("http3: unsupported frame")
	ErrInvalidFrameOrder  = errors.New("http3: invalid frame order")
	ErrHeaderListTooLarge = errors.New("http3: header list too large")
	ErrInvalidPipeline    = errors.New("http3: invalid pipeline config")
)
