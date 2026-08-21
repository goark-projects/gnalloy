//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package transport

import "github.com/goark-projects/gnalloy/transport/poller"

func newNativePoller(poller.Config) (Poller, error) {
	return nil, ErrUnsupportedPoller
}
