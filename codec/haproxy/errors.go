package haproxy

import "errors"

var (
	ErrInvalidFrame        = errors.New("haproxy: invalid frame")
	ErrUnsupportedProtocol = errors.New("haproxy: unsupported protocol")
	ErrInvalidTLV          = errors.New("haproxy: invalid tlv")
)
