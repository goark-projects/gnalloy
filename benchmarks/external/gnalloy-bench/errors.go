package main

import "errors"

var (
	errInvalidConfig       = errors.New("gnalloy-bench: invalid config")
	errUnsupportedProtocol = errors.New("gnalloy-bench: unsupported protocol")
	errInvalidBackend      = errors.New("gnalloy-bench: invalid backend")
)
