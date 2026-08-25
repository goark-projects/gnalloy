package quic

import "errors"

var (
	ErrInvalidVersion        = errors.New("gnalloy/transport/quic: invalid version")
	ErrInvalidConnectionID   = errors.New("gnalloy/transport/quic: invalid connection id")
	ErrInvalidConfig         = errors.New("gnalloy/transport/quic: invalid config")
	ErrInvalidPacket         = errors.New("gnalloy/transport/quic: invalid packet")
	ErrInvalidHeader         = errors.New("gnalloy/transport/quic: invalid header")
	ErrInvalidFrame          = errors.New("gnalloy/transport/quic: invalid frame")
	ErrInvalidVarInt         = errors.New("gnalloy/transport/quic: invalid variable-length integer")
	ErrConnectionNotFound    = errors.New("gnalloy/transport/quic: connection not found")
	ErrNotImplemented        = errors.New("gnalloy/transport/quic: protocol engine is not implemented yet")
	ErrUnsupportedCompletion = errors.New("gnalloy/transport/quic: completion poller packet ops are not implemented yet")
)
