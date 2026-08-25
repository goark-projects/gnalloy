package websocket

import "errors"

var (
	ErrInvalidHandshake       = errors.New("gnalloy/codec/websocket: invalid websocket handshake")
	ErrUnexpectedContinuation = errors.New("gnalloy/codec/websocket: unexpected continuation frame")
	ErrFragmentInProgress     = errors.New("gnalloy/codec/websocket: fragmented message already in progress")
	ErrControlFrameInvalid    = errors.New("gnalloy/codec/websocket: invalid control frame")
)
