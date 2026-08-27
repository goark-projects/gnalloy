//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package transport

import "goark.dev/gnalloy/transport/poller"

func newNativePoller(poller.Config) (Poller, error) {
	return nil, ErrUnsupportedPoller
}
