package tls

import "errors"

var (
	ErrNeedInput                       = errors.New("gnalloy/handler/tls: need more encrypted input")
	ErrInvalidConfig                   = errors.New("gnalloy/handler/tls: invalid config")
	ErrPeerCertificateUnavailable      = errors.New("gnalloy/handler/tls: peer certificate unavailable")
	ErrNativeTLSUnavailable            = errors.New("gnalloy/handler/tls: native tls provider unavailable")
	ErrInvalidClientHello              = errors.New("gnalloy/handler/tls: invalid client hello")
	ErrOCSPStapleRequired              = errors.New("gnalloy/handler/tls: ocsp staple required")
	ErrOCSPValidationFailed            = errors.New("gnalloy/handler/tls: ocsp validation failed")
	ErrUnknownCipherSuite              = errors.New("gnalloy/handler/tls: unknown cipher suite")
	ErrInsecureCipherSuite             = errors.New("gnalloy/handler/tls: insecure cipher suite")
	ErrTLS13CipherSuiteNotConfigurable = errors.New("gnalloy/handler/tls: tls 1.3 cipher suite is not configurable")
)
