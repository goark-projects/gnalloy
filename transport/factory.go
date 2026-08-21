package transport

import (
	"goark.dev/gnalloy/transport/poller"
	"goark.dev/gnalloy/transport/poller/memory"
)

func NewPoller(cfg Config) (Poller, error) {
	switch cfg.Backend {
	case BackendMemory:
		return memory.New(), nil
	default:
		return newNativePoller(poller.Config(cfg))
	}
}
