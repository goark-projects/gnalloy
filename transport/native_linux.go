//go:build linux

package transport

import (
	"goark.dev/gnalloy/transport/poller"
	"goark.dev/gnalloy/transport/poller/epoll"
	"goark.dev/gnalloy/transport/poller/iouring"
)

func newNativePoller(cfg poller.Config) (Poller, error) {
	switch cfg.Backend {
	case BackendEpoll:
		return epoll.New()
	case BackendIOUring:
		return iouring.NewWithConfig(iouring.Config{
			Entries:          cfg.Entries,
			SQPoll:           cfg.SQPoll,
			SQPollAffinity:   cfg.SQPollAffinity,
			SQPollCPU:        cfg.SQPollCPU,
			SQPollIdleMillis: cfg.SQPollIdleMillis,
			MultishotAccept:  cfg.MultishotAccept,
		})
	default:
		return nil, ErrUnsupportedPoller
	}
}
