package socks

import "errors"

var (
	ErrInvalidFrame   = errors.New("gnalloy/codec/socks: invalid frame")
	ErrInvalidAddress = errors.New("gnalloy/codec/socks: invalid address")
)
