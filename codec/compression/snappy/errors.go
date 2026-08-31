package snappy

import "errors"

var (
	ErrInvalidFrame      = errors.New("gnalloy/codec/compression/snappy: invalid framed chunk")
	ErrReservedChunkType = errors.New("gnalloy/codec/compression/snappy: reserved unskippable chunk type")
	ErrInvalidChecksum   = errors.New("gnalloy/codec/compression/snappy: invalid checksum")
)
