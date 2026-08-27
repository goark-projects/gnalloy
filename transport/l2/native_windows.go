//go:build windows

package l2

import (
	"context"
	"errors"
	"fmt"

	"goark.dev/gnalloy/transport/l2/internal/nativeframe"
)

type nativeDriver struct{}

// DefaultDriverKind 返回当前平台默认 L2 driver。
func DefaultDriverKind() DriverKind {
	return DriverKindNpcap
}

func (nativeDriver) Open(ctx context.Context, cfg Config) (Endpoint, error) {
	native, err := nativeframe.OpenNpcap(ctx, nativeframe.Config{
		InterfaceName:  cfg.InterfaceName,
		InterfaceIndex: cfg.InterfaceIndex,
		EtherType:      cfg.EtherType,
		Promiscuous:    cfg.Promiscuous,
		SnapshotLength: cfg.ReadBufferSize,
		Immediate:      true,
	})
	if err != nil {
		return nil, mapNativeDriverOpenError(err)
	}
	return newNativeFrameEndpoint(native), nil
}

func mapNativeDriverOpenError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, nativeframe.ErrInvalidConfig):
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	case errors.Is(err, nativeframe.ErrUnsupportedDriver), errors.Is(err, nativeframe.ErrUnavailable):
		return fmt.Errorf("%w: %v", ErrUnsupportedDriver, err)
	default:
		return err
	}
}
