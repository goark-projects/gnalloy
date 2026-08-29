//go:build linux

package iouring

import (
	"unsafe"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/poller"
)

func makeIOVectors(req poller.IORequest) ([]iovec, error) {
	vectors := make([]iovec, 0, iovCapacity(req))
	var err error
	if req.Buf != nil {
		vectors, err = appendIOVectors(vectors, req.Buf)
	} else {
		for _, buf := range req.Bufs {
			vectors, err = appendIOVectors(vectors, buf)
			if err != nil {
				return nil, err
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, poller.ErrInvalidIORequest
	}
	return vectors, nil
}

func iovCapacity(req poller.IORequest) int {
	if req.Buf != nil {
		return readableVectorCapacity(req.Buf)
	}
	n := 0
	for _, buf := range req.Bufs {
		n += readableVectorCapacity(buf)
	}
	if n == 0 {
		return 1
	}
	return n
}

func readableVectorCapacity(src buffer.ByteBuf) int {
	if src == nil || src.ReadableBytes() == 0 {
		return 0
	}
	if _, ok := buffer.ContiguousReadableBytes(src); ok {
		return 1
	}
	if composite, ok := src.(*buffer.CompositeByteBuf); ok {
		return composite.ComponentCount()
	}
	return 1
}

func appendIOVectors(dst []iovec, src buffer.ByteBuf) ([]iovec, error) {
	if src == nil || src.ReadableBytes() == 0 {
		return dst, nil
	}
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		return appendIovec(dst, data), nil
	}
	before := len(dst)
	buffer.ForEachReadableSlice(src, func(data []byte) bool {
		if len(data) == 0 {
			return true
		}
		dst = appendIovec(dst, data)
		return true
	})
	if len(dst) == before {
		return nil, poller.ErrInvalidIORequest
	}
	return dst, nil
}

func appendIovec(dst []iovec, data []byte) []iovec {
	return append(dst, iovec{
		base: uintptr(unsafe.Pointer(&data[0])),
		len:  uintptr(len(data)),
	})
}
