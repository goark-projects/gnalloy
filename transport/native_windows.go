//go:build windows

package transport

import (
	"goark.dev/gnalloy/transport/poller"
	"goark.dev/gnalloy/transport/poller/iocp"
)

func newNativePoller(cfg poller.Config) (Poller, error) {
	if cfg.Backend == BackendIOCP {
		return iocp.New()
	}
	return nil, ErrUnsupportedPoller
}
