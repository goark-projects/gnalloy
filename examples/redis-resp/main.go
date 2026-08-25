package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/redis"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9013")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	boss, workers, err := opts.NewGroups()
	if err != nil {
		fatal(err)
	}
	defer shutdown(boss)
	defer shutdown(workers)

	tcpConfig, err := opts.TCPConfig()
	if err != nil {
		fatal(err)
	}
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(tcpConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			frameDecoder, err := redis.NewFrameDecoder(1 << 20)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("encoder", redis.NewValueEncoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("frame", frameDecoder); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("value", redis.NewValueDecoder()); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("handler", redisHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy redis-resp listening on %s backend=%s boss=%d workers=%d reuseport=%v mmap=%v\n",
		server.Addr(), opts.BackendLabel(), opts.Boss, opts.Workers, opts.ReusePort, opts.Mmap)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type redisHandler struct{}

func (redisHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	value, ok := msg.(redis.Value)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer value.Release()

	if isPing(value) {
		if err := ctx.Channel().WriteAndFlush(redis.SimpleString{Value: "PONG"}); err != nil {
			ctx.FireExceptionCaught(err)
		}
		return
	}
	if err := ctx.Channel().WriteAndFlush(redis.Error{Value: "ERR unsupported command"}); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (redisHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}

func isPing(value redis.Value) bool {
	array, ok := value.(redis.Array)
	if !ok || len(array.Values) == 0 {
		return false
	}
	bulk, ok := array.Values[0].(redis.BulkString)
	if !ok || bulk.Data == nil {
		return false
	}
	return strings.EqualFold(string(bulk.Data.Bytes()), "PING")
}

func shutdown(group *transport.EventLoopGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = group.Shutdown(ctx)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
