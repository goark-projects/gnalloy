package timeout

import "errors"

var (
	ErrReadTimeout  = errors.New("gnalloy/handler/timeout: read timeout")
	ErrWriteTimeout = errors.New("gnalloy/handler/timeout: write timeout")
)
