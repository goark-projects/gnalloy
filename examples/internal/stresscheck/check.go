package stresscheck

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/examples/internal/stressclient"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

const ProtocolBoth stressclient.Protocol = "both"

var (
	ErrLeakDetected = errors.New("gnalloy/examples: stress leak detected")
	ErrCheckFailed  = errors.New("gnalloy/examples: stress check failed")
)

type Config struct {
	Addr            string
	Protocol        stressclient.Protocol
	Scenario        stressclient.Scenario
	Connections     int
	MessagesPerConn int
	PayloadSize     int
	Timeout         time.Duration
	Delay           time.Duration
	DrainTimeout    time.Duration

	BackendName string
	Boss        int
	Workers     int

	ReusePort bool

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

type Result struct {
	Requests int64
	Errors   int64
	Elapsed  time.Duration
}

// Run 在同一进程内启动服务端并执行压力客户端，随后检查连接和 allocator 泄漏。
func Run(ctx context.Context, cfg Config) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Protocol == "" {
		cfg.Protocol = ProtocolBoth
	}
	if cfg.Scenario == "" {
		cfg.Scenario = stressclient.ScenarioMixed
	}
	protocols, err := expandProtocols(cfg.Protocol)
	if err != nil {
		return Result{}, err
	}
	var out Result
	start := time.Now()
	for _, protocol := range protocols {
		result, err := runProtocol(ctx, cfg, protocol)
		out.Requests += result.TotalRequests
		out.Errors += result.Errors
		if err != nil {
			out.Elapsed = time.Since(start)
			return out, err
		}
	}
	out.Elapsed = time.Since(start)
	return out, nil
}

func expandProtocols(protocol stressclient.Protocol) ([]stressclient.Protocol, error) {
	switch protocol {
	case "", ProtocolBoth:
		return []stressclient.Protocol{stressclient.ProtocolRaw, stressclient.ProtocolLengthField}, nil
	case stressclient.ProtocolRaw, stressclient.ProtocolLengthField:
		return []stressclient.Protocol{protocol}, nil
	default:
		return nil, stressclient.ErrInvalidProtocol
	}
}

func runProtocol(ctx context.Context, cfg Config, protocol stressclient.Protocol) (stressclient.Result, error) {
	opts := buildOptions(cfg)
	opts.Addr = cfg.Addr
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:0"
	}
	if err := opts.Resolve(); err != nil {
		return stressclient.Result{}, err
	}
	boss, workers, err := opts.NewGroups()
	if err != nil {
		return stressclient.Result{}, err
	}
	defer shutdownGroup(workers)
	defer shutdownGroup(boss)

	tcpConfig, err := opts.TCPConfig()
	if err != nil {
		return stressclient.Result{}, err
	}
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(tcpConfig)).
		ChildInitializer(initializer(protocol)).
		BindContext(ctx, opts.Addr)
	if err != nil {
		return stressclient.Result{}, err
	}
	defer server.Close()

	result, err := stressclient.Run(ctx, stressclient.Config{
		Addr:            server.Addr(),
		Protocol:        protocol,
		Scenario:        cfg.Scenario,
		Connections:     cfg.Connections,
		MessagesPerConn: cfg.MessagesPerConn,
		PayloadSize:     cfg.PayloadSize,
		Timeout:         cfg.Timeout,
		Delay:           cfg.Delay,
	})
	if err != nil {
		return result, err
	}
	if err := waitNoActive(server, drainTimeout(cfg)); err != nil {
		return result, err
	}
	if err := checkAllocatorStats(server); err != nil {
		return result, err
	}
	if err := server.Close(); err != nil {
		return result, err
	}
	return result, nil
}

func buildOptions(cfg Config) *exampleconfig.Options {
	opts := &exampleconfig.Options{
		BackendName:             cfg.BackendName,
		Boss:                    cfg.Boss,
		Workers:                 cfg.Workers,
		ReusePort:               cfg.ReusePort,
		ReadBufferSize:          4096,
		Mmap:                    cfg.Mmap,
		MmapFallback:            cfg.MmapFallback,
		MmapBlockSize:           cfg.MmapBlockSize,
		MmapBlocks:              cfg.MmapBlocks,
		IOUringEntries:          cfg.IOUringEntries,
		IOUringSQPoll:           cfg.IOUringSQPoll,
		IOUringSQPollAffinity:   cfg.IOUringSQPollAffinity,
		IOUringSQPollCPU:        cfg.IOUringSQPollCPU,
		IOUringSQPollIdleMillis: cfg.IOUringSQPollIdleMillis,
		IOUringMultishotAccept:  cfg.IOUringMultishotAccept,
	}
	if opts.BackendName == "" {
		opts.BackendName = "default"
	}
	if opts.Boss <= 0 {
		opts.Boss = 1
	}
	if opts.Workers <= 0 {
		opts.Workers = 2
	}
	if opts.MmapBlockSize <= 0 {
		opts.MmapBlockSize = 4096
	}
	if opts.MmapBlocks <= 0 {
		opts.MmapBlocks = 4096
	}
	return opts
}

func initializer(protocol stressclient.Protocol) bootstrap.ChildInitializer {
	return func(ch channel.Channel) error {
		switch protocol {
		case stressclient.ProtocolRaw:
			return ch.Pipeline().AddLast("echo", echoHandler{})
		case stressclient.ProtocolLengthField:
			decoder, err := codec.NewLengthFieldBasedFrameDecoder(1<<20, 0, 4, 0, 4, buffer.BigEndian)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("frame", decoder); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("echo", lengthFieldEchoHandler{})
		default:
			return stressclient.ErrInvalidProtocol
		}
	}
}

func drainTimeout(cfg Config) time.Duration {
	if cfg.DrainTimeout > 0 {
		return cfg.DrainTimeout
	}
	if cfg.Timeout > 0 && cfg.Timeout < 5*time.Second {
		return cfg.Timeout
	}
	return 5 * time.Second
}

func waitNoActive(server bootstrap.Server, timeout time.Duration) error {
	observed, ok := server.(interface{ ActiveConnectionCount() int })
	if !ok {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for {
		if active := observed.ActiveConnectionCount(); active == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: active connections did not drain", ErrLeakDetected)
		}
		time.Sleep(time.Millisecond)
	}
}

func checkAllocatorStats(server bootstrap.Server) error {
	observed, ok := server.(interface {
		AllocatorStats() []buffer.AllocatorStats
	})
	if !ok {
		return nil
	}
	for i, stats := range observed.AllocatorStats() {
		if stats.InUse != 0 {
			return fmt.Errorf("%w: allocator[%d] in-use=%d free=%d", ErrLeakDetected, i, stats.InUse, stats.Free)
		}
	}
	return nil
}

func shutdownGroup(group *transport.EventLoopGroup) {
	if group == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = group.Shutdown(ctx)
}

type echoHandler struct{}

func (echoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (echoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}

type lengthFieldEchoHandler struct{}

func (lengthFieldEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer frame.Release()

	payload := frame.Bytes()
	out, err := ctx.Channel().Allocator().Acquire(4 + len(payload))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	binary.BigEndian.PutUint32(out.WritableBytesView()[:4], uint32(len(payload)))
	if err := out.AdvanceWriter(4); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := out.WriteBytes(payload); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Channel().WriteAndFlush(out); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (lengthFieldEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}
