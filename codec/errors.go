package codec

import "errors"

var (
	ErrInvalidLengthField = errors.New("gnalloy/codec: invalid length field")
	ErrFrameTooLong       = errors.New("gnalloy/codec: frame too long")
)
