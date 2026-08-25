package dns

import "errors"

var (
	ErrInvalidMessage  = errors.New("gnalloy/codec/dns: invalid message")
	ErrInvalidName     = errors.New("gnalloy/codec/dns: invalid name")
	ErrInvalidQuestion = errors.New("gnalloy/codec/dns: invalid question")
	ErrInvalidResource = errors.New("gnalloy/codec/dns: invalid resource")
)
