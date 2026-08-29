package main

import (
	"context"
	"fmt"
	"strconv"

	"goark.dev/gnalloy/benchmarks/external/internal/benchh2"
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http2"
	gnalloytls "goark.dev/gnalloy/handler/tls"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

const http2PrefaceHandlerName = "http2-preface"

func startHTTP2Server(ctx context.Context, cfg config) (*echoServer, error) {
	boss, workers, err := newGroups(cfg)
	if err != nil {
		return nil, err
	}
	server, err := bindHTTP2Server(ctx, cfg, boss, workers)
	if err != nil {
		shutdownGroups(boss, workers)
		return nil, err
	}
	return &echoServer{addr: server.Addr(), server: server, boss: boss, workers: workers}, nil
}

func bindHTTP2Server(ctx context.Context, cfg config, boss *transport.EventLoopGroup, workers *transport.EventLoopGroup) (bootstrap.Server, error) {
	tcpConfig := tcp.DefaultConfig()
	tcpConfig.ReadBufferSize = cfg.ReadBufferSize
	tcpConfig.ReusePort = cfg.ReusePort
	tcpConfig.IOUringFixedBuffers = cfg.IOUringFixedBuffers
	if cfg.Mmap {
		tcpConfig.AllocatorFactory = tcp.NewMmapAllocatorFactory(buffer.MmapAllocatorConfig{
			BlockSize: cfg.MmapBlockSize,
			Blocks:    cfg.MmapBlocks,
		}, false)
	}
	return bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(tcpConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			if cfg.Protocol == "https2" {
				tlsConfig, err := serverTLSConfig(cfg)
				if err != nil {
					return err
				}
				if err := ch.Pipeline().AddLast("tls", gnalloytls.Server(gnalloytls.Config{TLS: tlsConfig})); err != nil {
					return err
				}
			}
			return addHTTP2Pipeline(ch, cfg)
		}).
		BindContext(ctx, cfg.Addr)
}

func addHTTP2Pipeline(ch channel.Channel, cfg config) error {
	frameDecoder, err := http2.NewFrameDecoder(http2.DefaultMaxFrameSize)
	if err != nil {
		return err
	}
	headerDecoder, err := http2.NewHeaderDecoder(http2.HeaderCodecConfig{})
	if err != nil {
		return err
	}
	headerEncoder, err := http2.NewHeaderEncoder(http2.HeaderCodecConfig{})
	if err != nil {
		return err
	}
	mux, err := http2.NewStreamMultiplexer(http2.MultiplexerConfig{Server: true})
	if err != nil {
		return err
	}
	pipeline := ch.Pipeline()
	if err := pipeline.AddLast(http2PrefaceHandlerName, newHTTP2PrefaceDecoder()); err != nil {
		return err
	}
	if err := pipeline.AddLast("http2-frame-decoder", frameDecoder); err != nil {
		return err
	}
	if err := pipeline.AddLast("http2-typed-decoder", http2.NewTypedFrameDecoder()); err != nil {
		return err
	}
	frameEncoder := http2.NewFrameEncoder()
	if cfg.Protocol == "https2" {
		frameEncoder = http2.NewFrameEncoderWithConfig(http2.FrameEncoderConfig{CoalescePayload: true})
	}
	if err := pipeline.AddLast("http2-frame-encoder", frameEncoder); err != nil {
		return err
	}
	if err := pipeline.AddLast("http2-typed-encoder", http2.NewTypedFrameEncoder()); err != nil {
		return err
	}
	if err := pipeline.AddLast("http2-header-decoder", headerDecoder); err != nil {
		return err
	}
	if err := pipeline.AddLast("http2-header-encoder", headerEncoder); err != nil {
		return err
	}
	if err := pipeline.AddLast("http2-mux", mux); err != nil {
		return err
	}
	return pipeline.AddLast("http2-handler", newHTTP2BenchmarkHandler(cfg.Payload))
}

type http2PrefaceDecoder struct {
	*codec.ByteToMessageDecoder
	matched int
	done    bool
}

func newHTTP2PrefaceDecoder() *http2PrefaceDecoder {
	d := &http2PrefaceDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *http2PrefaceDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if d.done {
		return d.sliceReadable(in)
	}
	preface := []byte(http2.ClientPreface)
	needed := len(preface) - d.matched
	readable := in.ReadableBytes()
	if readable < needed {
		if err := d.matchPreface(in, readable); err != nil {
			return nil, err
		}
		d.matched += readable
		return nil, in.SkipBytes(readable)
	}
	if err := d.matchPreface(in, needed); err != nil {
		return nil, err
	}
	d.matched = len(preface)
	d.done = true
	if err := in.SkipBytes(needed); err != nil {
		return nil, err
	}
	return d.sliceReadable(in)
}

func (d *http2PrefaceDecoder) matchPreface(in *buffer.CompositeByteBuf, n int) error {
	preface := []byte(http2.ClientPreface)
	index := in.ReaderIndex()
	for i := 0; i < n; i++ {
		b, ok := in.GetByte(index + i)
		if !ok || b != preface[d.matched+i] {
			return fmt.Errorf("gnalloy-bench: invalid http2 client preface")
		}
	}
	return nil
}

func (d *http2PrefaceDecoder) sliceReadable(in *buffer.CompositeByteBuf) (buffer.ByteBuf, error) {
	readable := in.ReadableBytes()
	if readable == 0 {
		return nil, nil
	}
	out, err := in.Slice(in.ReaderIndex(), readable)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(readable); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

type http2BenchmarkHandler struct {
	body   []byte
	fields []http2.HeaderField
}

func newHTTP2BenchmarkHandler(payload int) http2BenchmarkHandler {
	body := benchh2.ResponseBody(payload)
	return http2BenchmarkHandler{
		body: body,
		fields: []http2.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-type", Value: "application/octet-stream"},
			{Name: "content-length", Value: strconv.Itoa(len(body))},
		},
	}
}

func (h http2BenchmarkHandler) ChannelActive(ctx *channel.HandlerContext) {
	if err := ctx.WriteAndFlush(http2.SettingsFrame{}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

func (h http2BenchmarkHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case http2.SettingsFrame:
		if !frame.Ack {
			if err := ctx.WriteAndFlush(http2.SettingsFrame{Ack: true}); err != nil {
				ctx.FireExceptionCaught(err)
			}
		}
	case http2.PingFrame:
		if !frame.Ack {
			frame.Ack = true
			if err := ctx.WriteAndFlush(frame); err != nil {
				ctx.FireExceptionCaught(err)
			}
		}
	case http2.StreamEvent:
		defer frame.Release()
		if frame.Type != http2.StreamEventRead {
			return
		}
		if _, ok := frame.Frame.(http2.HeadersBlock); !ok {
			return
		}
		if err := h.writeResponse(ctx, frame.StreamID); err != nil {
			ctx.FireExceptionCaught(err)
		}
	default:
		ctx.FireChannelRead(msg)
	}
}

func (h http2BenchmarkHandler) writeResponse(ctx *channel.HandlerContext, streamID http2.StreamID) error {
	if err := ctx.Write(http2.HeadersBlock{StreamID: streamID, Fields: h.fields}); err != nil {
		return err
	}
	body, err := ctx.Channel().Allocator().Acquire(len(h.body))
	if err != nil {
		return err
	}
	if _, err := body.WriteBytes(h.body); err != nil {
		body.Release()
		return err
	}
	if err := ctx.Write(http2.DataFrame{StreamID: streamID, Flags: http2.FlagEndStream, Data: body}); err != nil {
		return err
	}
	return ctx.Flush()
}

func (http2BenchmarkHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Close()
}
