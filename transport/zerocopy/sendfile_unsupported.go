//go:build !linux

package zerocopy

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func sendFile(transport.FDRef, channel.FileRegion, int) (Result, bool, error) {
	return Result{}, false, ErrUnsupported
}
