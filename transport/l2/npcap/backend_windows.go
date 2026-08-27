//go:build windows

package npcap

import (
	"context"
	"errors"
	"fmt"

	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/l2/internal/nativeframe"
)

type nativeBackend struct{}

func defaultBackend() Backend {
	return nativeBackend{}
}

func (nativeBackend) OpenNpcap(ctx context.Context, cfg Config) (l2.Endpoint, error) {
	native, err := nativeframe.OpenNpcap(ctx, nativeframe.Config{
		InterfaceName:  cfg.InterfaceName,
		InterfaceIndex: cfg.InterfaceIndex,
		EtherType:      cfg.EtherType,
		Promiscuous:    cfg.Promiscuous,
		SnapshotLength: cfg.SnapshotLength,
		Immediate:      cfg.Immediate,
		ReadTimeout:    cfg.ReadTimeout,
		BufferSize:     cfg.BufferSize,
	})
	if err != nil {
		return nil, wrapNativeOpenError(err)
	}
	return newNativeEndpoint(native), nil
}

func wrapNativeOpenError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, nativeframe.ErrInvalidConfig):
		return fmt.Errorf("%w: %w", l2.ErrInvalidConfig, ErrInvalidConfig)
	case errors.Is(err, nativeframe.ErrUnavailable), errors.Is(err, nativeframe.ErrUnsupportedDriver):
		return fmt.Errorf("%w: %w", l2.ErrUnsupportedDriver, ErrUnavailable)
	default:
		return err
	}
}
