package dns

import "errors"

var (
	ErrNoAnswer      = errors.New("gnalloy/resolver/dns: no answer")
	ErrInvalidReply  = errors.New("gnalloy/resolver/dns: invalid reply")
	ErrNoNameServer  = errors.New("gnalloy/resolver/dns: no name server")
	ErrServerFailure = errors.New("gnalloy/resolver/dns: name server failure")
)
