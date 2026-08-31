package lzf

import "errors"

var (
	ErrCorruptFrame       = errors.New("gnalloy/codec/compression/lzf: corrupt frame")
	ErrInsufficientBuffer = errors.New("gnalloy/codec/compression/lzf: insufficient buffer")
)
