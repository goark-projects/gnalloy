package pool

import "errors"

var (
	ErrInvalidConfig  = errors.New("gnalloy/channel/pool: invalid config")
	ErrClosedPool     = errors.New("gnalloy/channel/pool: closed pool")
	ErrInvalidChannel = errors.New("gnalloy/channel/pool: invalid channel")
)
