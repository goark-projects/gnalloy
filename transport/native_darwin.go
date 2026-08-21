//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package transport

import (
	"github.com/goark-projects/gnalloy/transport/poller"
	"github.com/goark-projects/gnalloy/transport/poller/kqueue"
)

func newNativePoller(cfg poller.Config) (Poller, error) {
	if cfg.Backend == BackendKqueue {
		return kqueue.New()
	}
	return nil, ErrUnsupportedPoller
}
