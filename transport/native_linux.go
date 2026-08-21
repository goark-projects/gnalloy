//go:build linux

package transport

import (
	"github.com/goark-projects/gnalloy/transport/poller"
	"github.com/goark-projects/gnalloy/transport/poller/epoll"
	"github.com/goark-projects/gnalloy/transport/poller/iouring"
)

func newNativePoller(cfg poller.Config) (Poller, error) {
	switch cfg.Backend {
	case BackendEpoll:
		return epoll.New()
	case BackendIOUring:
		return iouring.New()
	default:
		return nil, ErrUnsupportedPoller
	}
}
