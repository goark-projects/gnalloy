package stomp

import "errors"

var (
	ErrInvalidFrame  = errors.New("gnalloy/codec/stomp: invalid frame")
	ErrInvalidHeader = errors.New("gnalloy/codec/stomp: invalid header")
)
