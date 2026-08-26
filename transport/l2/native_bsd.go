//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package l2

import "context"

type nativeDriver struct{}

// DefaultDriverKind 返回当前平台默认 L2 driver。
func DefaultDriverKind() DriverKind {
	return DriverKindBPF
}

func (nativeDriver) Open(context.Context, Config) (Endpoint, error) {
	return nil, ErrUnsupportedDriver
}
