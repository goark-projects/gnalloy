package main

import (
	"fmt"
	"strings"
)

const (
	defaultHTTP1Mode = http1ModeCodec

	http1ModeCodec = "codec"
	http1ModeRaw   = "raw"
)

func normalizeHTTP1Mode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", http1ModeCodec:
		return http1ModeCodec, nil
	case http1ModeRaw:
		return http1ModeRaw, nil
	default:
		return "", fmt.Errorf("%w: invalid http1-mode %q", errInvalidConfig, value)
	}
}
