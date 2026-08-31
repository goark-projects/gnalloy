package local

import "errors"

var (
	ErrAddressInUse   = errors.New("gnalloy/transport/local: address already in use")
	ErrServerNotFound = errors.New("gnalloy/transport/local: server not found")
	ErrClosed         = errors.New("gnalloy/transport/local: endpoint closed")
	ErrNotConnected   = errors.New("gnalloy/transport/local: endpoint is not connected")
)
