package l2

import (
	"context"
	"errors"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/l2/internal/nativeframe"
)

type nativeFrameEndpoint struct {
	native nativeframe.Endpoint
}

func newNativeFrameEndpoint(native nativeframe.Endpoint) Endpoint {
	return &nativeFrameEndpoint{native: native}
}

func (e *nativeFrameEndpoint) Addr() string {
	if e == nil || e.native == nil {
		return ""
	}
	return e.native.Addr()
}

func (e *nativeFrameEndpoint) ReadFrame(ctx context.Context, alloc buffer.Allocator, readBufferSize int) (Frame, error) {
	if e == nil || e.native == nil || alloc == nil {
		return Frame{}, ErrInvalidConfig
	}
	frame, err := e.native.ReadFrame(ctx, readBufferSize)
	if err != nil {
		return Frame{}, mapNativeFrameError(err)
	}
	payload, err := alloc.Acquire(len(frame.Data))
	if err != nil {
		return Frame{}, err
	}
	if _, err := payload.WriteBytes(frame.Data); err != nil {
		payload.Release()
		return Frame{}, err
	}
	return Frame{Meta: frameMetaFromNative(frame.Meta), Payload: payload}, nil
}

func (e *nativeFrameEndpoint) WriteFrame(ctx context.Context, frame Frame) error {
	if e == nil || e.native == nil {
		return ErrInvalidConfig
	}
	if !frame.Valid() {
		return ErrInvalidFrame
	}
	if err := e.native.WriteFrame(ctx, frame.Payload.Bytes()); err != nil {
		return mapNativeFrameError(err)
	}
	return nil
}

func (e *nativeFrameEndpoint) Close() error {
	if e == nil || e.native == nil {
		return nil
	}
	return e.native.Close()
}

func frameMetaFromNative(meta nativeframe.Meta) FrameMeta {
	return FrameMeta{
		InterfaceName:  meta.InterfaceName,
		InterfaceIndex: meta.InterfaceIndex,
		Source:         meta.Source,
		Destination:    meta.Destination,
		EtherType:      meta.EtherType,
	}
}

func mapNativeFrameError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, nativeframe.ErrInvalidConfig):
		return ErrInvalidConfig
	case errors.Is(err, nativeframe.ErrUnsupportedDriver), errors.Is(err, nativeframe.ErrUnavailable):
		return ErrUnsupportedDriver
	case errors.Is(err, nativeframe.ErrInvalidFrame):
		return ErrInvalidFrame
	case errors.Is(err, nativeframe.ErrClosed):
		return ErrClosed
	default:
		return err
	}
}
