package defense

import "errors"

var (
	ErrTooManyRSTFrames     = errors.New("gnalloy/codec/http2/defense: too many rst_stream frames")
	ErrTooManyControlFrames = errors.New("gnalloy/codec/http2/defense: too many queued control frames")
)
