package npcap

import (
	"context"
	"errors"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/l2/internal/nativeframe"
)

type nativeEndpoint struct {
	native nativeframe.Endpoint
}

func newNativeEndpoint(native nativeframe.Endpoint) l2.Endpoint {
	return &nativeEndpoint{native: native}
}

func (e *nativeEndpoint) Addr() string {
	if e == nil || e.native == nil {
		return ""
	}
	return e.native.Addr()
}

func (e *nativeEndpoint) ReadFrame(ctx context.Context, alloc buffer.Allocator, readBufferSize int) (l2.Frame, error) {
	if e == nil || e.native == nil || alloc == nil {
		return l2.Frame{}, l2.ErrInvalidConfig
	}
	frame, err := e.native.ReadFrame(ctx, readBufferSize)
	if err != nil {
		return l2.Frame{}, mapNativeEndpointError(err)
	}
	payload, err := alloc.Acquire(len(frame.Data))
	if err != nil {
		return l2.Frame{}, err
	}
	if _, err := payload.WriteBytes(frame.Data); err != nil {
		payload.Release()
		return l2.Frame{}, err
	}
	return l2.Frame{Meta: toL2Meta(frame.Meta), Payload: payload}, nil
}

func (e *nativeEndpoint) WriteFrame(ctx context.Context, frame l2.Frame) error {
	if e == nil || e.native == nil {
		return l2.ErrInvalidConfig
	}
	if !frame.Valid() {
		return l2.ErrInvalidFrame
	}
	return mapNativeEndpointError(e.native.WriteFrame(ctx, frame.Payload.Bytes()))
}

func (e *nativeEndpoint) Close() error {
	if e == nil || e.native == nil {
		return nil
	}
	return e.native.Close()
}

func toL2Meta(meta nativeframe.Meta) l2.FrameMeta {
	return l2.FrameMeta{
		InterfaceName:  meta.InterfaceName,
		InterfaceIndex: meta.InterfaceIndex,
		Source:         meta.Source,
		Destination:    meta.Destination,
		EtherType:      meta.EtherType,
	}
}

func mapNativeEndpointError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, nativeframe.ErrInvalidConfig):
		return l2.ErrInvalidConfig
	case errors.Is(err, nativeframe.ErrInvalidFrame):
		return l2.ErrInvalidFrame
	case errors.Is(err, nativeframe.ErrClosed):
		return l2.ErrClosed
	case errors.Is(err, nativeframe.ErrUnavailable), errors.Is(err, nativeframe.ErrUnsupportedDriver):
		return l2.ErrUnsupportedDriver
	default:
		return err
	}
}
