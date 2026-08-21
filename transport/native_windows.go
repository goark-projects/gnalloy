//go:build windows

package transport

import (
	"github.com/goark-projects/gnalloy/transport/poller"
	"github.com/goark-projects/gnalloy/transport/poller/iocp"
)

func newNativePoller(cfg poller.Config) (Poller, error) {
	if cfg.Backend == BackendIOCP {
		return iocp.New()
	}
	return nil, ErrUnsupportedPoller
}
