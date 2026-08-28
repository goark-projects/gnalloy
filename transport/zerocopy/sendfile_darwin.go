//go:build darwin

package zerocopy

import (
	"errors"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

func sendFile(dst transport.FDRef, region channel.FileRegion, chunkSize int) (Result, bool, error) {
	src, offset, advance, ok := nativeFile(region)
	if !ok {
		return Result{}, false, ErrUnsupported
	}
	remaining := transferSize(region, chunkSize)
	sendOffset := offset + region.Transferred()
	n, err := unix.Sendfile(dst.FD, int(src.Fd()), &sendOffset, int(remaining))
	result := Result{Bytes: int64(n), ZeroCopy: n > 0}
	if n > 0 {
		if advanceErr := advance(int64(n)); advanceErr != nil {
			return result, false, advanceErr
		}
	}
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return result, true, nil
	}
	if err != nil {
		return result, false, err
	}
	return result, false, nil
}
