package macos

import "errors"

var (
	ErrUnsupportedPlatform = errors.New("gnalloy/resolver/dns/native/macos: unsupported platform")
	ErrInvalidConfig       = errors.New("gnalloy/resolver/dns/native/macos: invalid resolver config")
)
