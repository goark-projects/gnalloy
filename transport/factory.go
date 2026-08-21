package transport

import (
	"github.com/goark-projects/gnalloy/transport/poller"
	"github.com/goark-projects/gnalloy/transport/poller/memory"
)

func NewPoller(cfg Config) (Poller, error) {
	switch cfg.Backend {
	case BackendMemory:
		return memory.New(), nil
	default:
		return newNativePoller(poller.Config(cfg))
	}
}
