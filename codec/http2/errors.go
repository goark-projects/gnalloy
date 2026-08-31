package http2

import "errors"

var (
	ErrInvalidFrame       = errors.New("gnalloy/codec/http2: invalid frame")
	ErrFrameTooLarge      = errors.New("gnalloy/codec/http2: frame too large")
	ErrInvalidStreamID    = errors.New("gnalloy/codec/http2: invalid stream id")
	ErrInvalidStreamState = errors.New("gnalloy/codec/http2: invalid stream state")
	ErrFlowControl        = errors.New("gnalloy/codec/http2: flow control violation")
	ErrHeaderBlock        = errors.New("gnalloy/codec/http2: invalid header block")
	ErrHeaderListTooLarge = errors.New("gnalloy/codec/http2: header list too large")
	ErrMissingChildInit   = errors.New("gnalloy/codec/http2: child initializer is required")
	ErrChildClosed        = errors.New("gnalloy/codec/http2: child channel closed")
	ErrChildMessage       = errors.New("gnalloy/codec/http2: unsupported child channel message")
	ErrStreamBufferFull   = errors.New("gnalloy/codec/http2: stream buffer full")
)
