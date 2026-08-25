package compression

import "errors"

var (
	ErrInvalidConfig  = errors.New("gnalloy/codec/compression: invalid config")
	ErrDecodedTooLong = errors.New("gnalloy/codec/compression: decoded payload too long")
)
