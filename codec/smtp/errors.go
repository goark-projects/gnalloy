package smtp

import "errors"

var (
	ErrInvalidLine     = errors.New("gnalloy/codec/smtp: invalid line")
	ErrInvalidResponse = errors.New("gnalloy/codec/smtp: invalid response")
	ErrInvalidRequest  = errors.New("gnalloy/codec/smtp: invalid request")
)
