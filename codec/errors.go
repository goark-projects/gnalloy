package codec

import "errors"

var (
	ErrInvalidLengthField = errors.New("gnalloy/codec: invalid length field")
	ErrFrameTooLong       = errors.New("gnalloy/codec: frame too long")
	ErrInvalidFrameLength = errors.New("gnalloy/codec: invalid frame length")
	ErrInvalidDelimiter   = errors.New("gnalloy/codec: invalid delimiter")
	ErrInvalidDecoder     = errors.New("gnalloy/codec: invalid byte decoder")
	ErrInvalidEncoder     = errors.New("gnalloy/codec: invalid byte encoder")
	ErrDecoderNoProgress  = errors.New("gnalloy/codec: decoder emitted message without consuming bytes")
	ErrEncodedLengthRange = errors.New("gnalloy/codec: encoded length out of range")
)
