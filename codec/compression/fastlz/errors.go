package fastlz

import "errors"

var (
	ErrCorruptFrame     = errors.New("gnalloy/codec/compression/fastlz: corrupt frame")
	ErrChecksumMismatch = errors.New("gnalloy/codec/compression/fastlz: checksum mismatch")
)
