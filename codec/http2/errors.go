package http2

import "errors"

var (
	ErrInvalidFrame       = errors.New("gnalloy/codec/http2: invalid frame")
	ErrFrameTooLarge      = errors.New("gnalloy/codec/http2: frame too large")
	ErrInvalidStreamID    = errors.New("gnalloy/codec/http2: invalid stream id")
	ErrInvalidStreamState = errors.New("gnalloy/codec/http2: invalid stream state")
	ErrFlowControl        = errors.New("gnalloy/codec/http2: flow control violation")
)
