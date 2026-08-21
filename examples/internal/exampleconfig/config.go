package exampleconfig

import (
	"errors"
	"flag"
	"fmt"
	"runtime"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

var (
	ErrInvalidBackend = errors.New("gnalloy/examples: invalid backend")
	ErrInvalidConfig  = errors.New("gnalloy/examples: invalid config")
)

// Options 保存示例服务端可调参数，方便跨平台验证不同 I/O 后端。
type Options struct {
	Addr        string
	BackendName string
	Backend     transport.BackendKind

	Boss    int
	Workers int

	ReusePort      bool
	ReadBufferSize int

	Mmap          bool
	MmapFallback  bool
	MmapBlockSize int
	MmapBlocks    int

	IOUringEntries          uint
	IOUringSQPoll           bool
	IOUringSQPollAffinity   bool
	IOUringSQPollCPU        int
	IOUringSQPollIdleMillis uint
	IOUringMultishotAccept  bool
}

// Register 把示例通用参数注册到指定 FlagSet。
func Register(fs *flag.FlagSet, defaultAddr string) *Options {
	opts := &Options{
		Addr:           defaultAddr,
		BackendName:    "default",
		Boss:           1,
		Workers:        runtime.GOMAXPROCS(0),
		ReadBufferSize: 4096,
		MmapFallback:   true,
		MmapBlockSize:  4096,
		MmapBlocks:     4096,
	}

	fs.StringVar(&opts.Addr, "addr", opts.Addr, "listen address")
	fs.StringVar(&opts.BackendName, "backend", opts.BackendName, "poller backend: default, epoll, iouring, kqueue, iocp, memory")
	fs.IntVar(&opts.Boss, "boss", opts.Boss, "boss event loop count")
	fs.IntVar(&opts.Workers, "workers", opts.Workers, "worker event loop count")
	fs.BoolVar(&opts.ReusePort, "reuseport", opts.ReusePort, "enable SO_REUSEPORT when supported")
	fs.IntVar(&opts.ReadBufferSize, "read-buffer-size", opts.ReadBufferSize, "per-read ByteBuf size")

	fs.BoolVar(&opts.Mmap, "mmap", opts.Mmap, "use per-worker mmap allocator")
	fs.BoolVar(&opts.MmapFallback, "mmap-fallback", opts.MmapFallback, "fallback to heap allocator when mmap is unsupported")
	fs.IntVar(&opts.MmapBlockSize, "mmap-block-size", opts.MmapBlockSize, "mmap allocator block size")
	fs.IntVar(&opts.MmapBlocks, "mmap-blocks", opts.MmapBlocks, "mmap allocator block count per worker")

	fs.UintVar(&opts.IOUringEntries, "iouring-entries", opts.IOUringEntries, "io_uring queue depth, 0 means backend default")
	fs.BoolVar(&opts.IOUringSQPoll, "iouring-sqpoll", opts.IOUringSQPoll, "enable io_uring SQPOLL")
	fs.BoolVar(&opts.IOUringSQPollAffinity, "iouring-sqpoll-affinity", opts.IOUringSQPollAffinity, "pin io_uring SQPOLL kernel thread")
	fs.IntVar(&opts.IOUringSQPollCPU, "iouring-sqpoll-cpu", opts.IOUringSQPollCPU, "io_uring SQPOLL CPU id")
	fs.UintVar(&opts.IOUringSQPollIdleMillis, "iouring-sqpoll-idle-ms", opts.IOUringSQPollIdleMillis, "io_uring SQPOLL idle timeout in milliseconds")
	fs.BoolVar(&opts.IOUringMultishotAccept, "iouring-multishot-accept", opts.IOUringMultishotAccept, "enable io_uring multishot accept")
	return opts
}

// Resolve 校验并固化参数，main 在创建资源前应调用一次。
func (o *Options) Resolve() error {
	if o == nil {
		return ErrInvalidConfig
	}
	backend, err := ParseBackend(o.BackendName)
	if err != nil {
		return err
	}
	if o.Boss <= 0 {
		return fmt.Errorf("%w: boss must be positive", ErrInvalidConfig)
	}
	if o.Workers <= 0 {
		return fmt.Errorf("%w: workers must be positive", ErrInvalidConfig)
	}
	if o.ReadBufferSize <= 0 {
		return fmt.Errorf("%w: read-buffer-size must be positive", ErrInvalidConfig)
	}
	if o.Mmap && (o.MmapBlockSize <= 0 || o.MmapBlocks <= 0) {
		return fmt.Errorf("%w: mmap block size and blocks must be positive", ErrInvalidConfig)
	}
	if o.IOUringSQPollAffinity && !o.IOUringSQPoll {
		return fmt.Errorf("%w: iouring-sqpoll-affinity requires iouring-sqpoll", ErrInvalidConfig)
	}
	if o.IOUringSQPollCPU < 0 {
		return fmt.Errorf("%w: iouring-sqpoll-cpu must be non-negative", ErrInvalidConfig)
	}
	o.Backend = backend
	return nil
}

func (o *Options) NewGroups() (*transport.EventLoopGroup, *transport.EventLoopGroup, error) {
	if err := o.Resolve(); err != nil {
		return nil, nil, err
	}
	pollerConfig := o.PollerConfig()
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         o.Boss,
		PollerConfig: pollerConfig,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create boss group: %w", err)
	}
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         o.Workers,
		PollerConfig: pollerConfig,
	})
	if err != nil {
		_ = boss.Close()
		return nil, nil, fmt.Errorf("create worker group: %w", err)
	}
	return boss, workers, nil
}

func (o *Options) PollerConfig() transport.Config {
	return transport.Config{
		Backend:          o.Backend,
		Entries:          uint32(o.IOUringEntries),
		SQPoll:           o.IOUringSQPoll,
		SQPollAffinity:   o.IOUringSQPollAffinity,
		SQPollCPU:        o.IOUringSQPollCPU,
		SQPollIdleMillis: uint32(o.IOUringSQPollIdleMillis),
		MultishotAccept:  o.IOUringMultishotAccept,
	}
}

func (o *Options) TCPConfig() (tcp.Config, error) {
	if err := o.Resolve(); err != nil {
		return tcp.Config{}, err
	}
	cfg := tcp.DefaultConfig()
	cfg.ReusePort = o.ReusePort
	cfg.ReadBufferSize = o.ReadBufferSize
	if o.Mmap {
		cfg.AllocatorFactory = tcp.NewMmapAllocatorFactory(buffer.MmapAllocatorConfig{
			BlockSize: o.MmapBlockSize,
			Blocks:    o.MmapBlocks,
		}, o.MmapFallback)
	}
	return cfg, nil
}

func (o *Options) BackendLabel() string {
	return BackendLabel(o.Backend)
}

func ParseBackend(name string) (transport.BackendKind, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "default":
		return transport.DefaultBackend(), nil
	case "memory":
		return transport.BackendMemory, nil
	case "epoll":
		return transport.BackendEpoll, nil
	case "iouring", "io_uring":
		return transport.BackendIOUring, nil
	case "kqueue":
		return transport.BackendKqueue, nil
	case "iocp":
		return transport.BackendIOCP, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrInvalidBackend, name)
	}
}

func BackendLabel(backend transport.BackendKind) string {
	switch backend {
	case transport.BackendMemory:
		return "memory"
	case transport.BackendStd:
		return "std"
	case transport.BackendEpoll:
		return "epoll"
	case transport.BackendKqueue:
		return "kqueue"
	case transport.BackendIOUring:
		return "iouring"
	case transport.BackendIOCP:
		return "iocp"
	default:
		return "unknown"
	}
}
