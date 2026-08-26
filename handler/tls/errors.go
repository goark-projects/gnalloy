package tls

import "errors"

var (
	ErrNeedInput                  = errors.New("gnalloy/handler/tls: need more encrypted input")
	ErrInvalidConfig              = errors.New("gnalloy/handler/tls: invalid config")
	ErrPeerCertificateUnavailable = errors.New("gnalloy/handler/tls: peer certificate unavailable")
)
