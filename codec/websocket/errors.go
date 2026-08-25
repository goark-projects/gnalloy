package websocket

import "errors"

var (
	ErrInvalidHandshake         = errors.New("gnalloy/codec/websocket: invalid websocket handshake")
	ErrUnexpectedContinuation   = errors.New("gnalloy/codec/websocket: unexpected continuation frame")
	ErrFragmentInProgress       = errors.New("gnalloy/codec/websocket: fragmented message already in progress")
	ErrControlFrameInvalid      = errors.New("gnalloy/codec/websocket: invalid control frame")
	ErrMaskMismatch             = errors.New("gnalloy/codec/websocket: websocket mask policy mismatch")
	ErrCloseStatusInvalid       = errors.New("gnalloy/codec/websocket: invalid close status")
	ErrInvalidUTF8              = errors.New("gnalloy/codec/websocket: invalid utf-8 text frame")
	ErrCloseHandshakeInProgress = errors.New("gnalloy/codec/websocket: close handshake in progress")
)
