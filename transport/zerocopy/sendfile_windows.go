//go:build windows

package zerocopy

import (
	"errors"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/windows"
)

const maxTransmitFileChunk = 0x7fffffff - 1

func sendFile(dst transport.FDRef, region channel.FileRegion, chunkSize int) (Result, bool, error) {
	src, offset, advance, ok := nativeFile(region)
	if !ok {
		return Result{}, false, ErrUnsupported
	}
	remaining := transferSize(region, chunkSize)
	if remaining > maxTransmitFileChunk {
		remaining = maxTransmitFileChunk
	}
	socket := windows.Handle(uintptr(dst.FD))
	file := windows.Handle(src.Fd())
	overlapped := windows.Overlapped{}
	start := offset + region.Transferred()
	overlapped.Offset = uint32(start)
	overlapped.OffsetHigh = uint32(uint64(start) >> 32)

	err := windows.TransmitFile(socket, file, uint32(remaining), 0, &overlapped, nil, windows.TF_WRITE_BEHIND)
	if err == nil {
		result := Result{Bytes: remaining, ZeroCopy: remaining > 0}
		if remaining > 0 {
			if advanceErr := advance(remaining); advanceErr != nil {
				return result, false, advanceErr
			}
		}
		return result, false, nil
	}
	if errors.Is(err, windows.WSAEWOULDBLOCK) {
		return Result{}, true, nil
	}
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		return waitTransmitFile(socket, &overlapped, advance)
	}
	if errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
		return Result{}, false, ErrUnsupported
	}
	return Result{}, false, err
}

func waitTransmitFile(socket windows.Handle, overlapped *windows.Overlapped, advance func(int64) error) (Result, bool, error) {
	var done uint32
	var flags uint32
	err := windows.WSAGetOverlappedResult(socket, overlapped, &done, true, &flags)
	result := Result{Bytes: int64(done), ZeroCopy: done > 0}
	if done > 0 {
		if advanceErr := advance(int64(done)); advanceErr != nil {
			return result, false, advanceErr
		}
	}
	if errors.Is(err, windows.WSAEWOULDBLOCK) || errors.Is(err, windows.ERROR_IO_INCOMPLETE) {
		return result, true, nil
	}
	if err != nil {
		return result, false, err
	}
	return result, false, nil
}
