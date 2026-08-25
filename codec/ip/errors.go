package ip

import "errors"

var (
	ErrInvalidPacket   = errors.New("gnalloy/codec/ip: invalid packet")
	ErrInvalidHeader   = errors.New("gnalloy/codec/ip: invalid header")
	ErrInvalidProtocol = errors.New("gnalloy/codec/ip: invalid protocol")
	ErrInvalidAddress  = errors.New("gnalloy/codec/ip: invalid address")
)
