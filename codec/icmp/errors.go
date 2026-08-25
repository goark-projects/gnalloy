package icmp

import "errors"

var (
	ErrInvalidMessage       = errors.New("gnalloy/codec/icmp: invalid message")
	ErrUnsupportedProtocol  = errors.New("gnalloy/codec/icmp: unsupported protocol")
	ErrMissingIPv6PseudoHdr = errors.New("gnalloy/codec/icmp: missing icmpv6 pseudo header source ip")
)
