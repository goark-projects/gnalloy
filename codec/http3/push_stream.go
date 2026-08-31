package http3

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// PushIDFrame 表示 HTTP/3 push stream type 后面的 push ID 前缀。
type PushIDFrame struct {
	PushID uint64
}

// Release 保持 PushIDFrame 可进入统一释放路径。
func (PushIDFrame) Release() {}

// PushIDDecoder 从 push stream 前缀中解析 push ID。
type PushIDDecoder struct {
	*codec.ByteToMessageDecoder
	seen bool
}

// PushIDFrameEncoder 将 PushIDFrame 编码为 HTTP/3 变长整数。
type PushIDFrameEncoder struct{}

// LocalPushIDHandler 在本端 push stream 激活时写出 push ID 前缀。
type LocalPushIDHandler struct {
	pushID uint64
	wrote  bool
}

// NewPushIDDecoder 创建 push ID 前缀解码器。
func NewPushIDDecoder() *PushIDDecoder {
	d := &PushIDDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageListDecoder(d)
	return d
}

// NewPushIDFrameEncoder 创建 push ID 前缀编码器。
func NewPushIDFrameEncoder() *PushIDFrameEncoder {
	return &PushIDFrameEncoder{}
}

// NewLocalPushIDHandler 创建本端 push stream 前缀写入 handler。
func NewLocalPushIDHandler(pushID uint64) (*LocalPushIDHandler, error) {
	if pushID > maxVarInt {
		return nil, ErrInvalidVarInt
	}
	return &LocalPushIDHandler{pushID: pushID}, nil
}

// DecodeBytes 解码 push ID，并把剩余 frame 字节交给后续 HTTP/3 frame decoder。
func (d *PushIDDecoder) DecodeBytes(_ *channel.HandlerContext, in *buffer.CompositeByteBuf, out *codec.MessageList) error {
	if !d.seen {
		pushID, n, ok, err := readVarInt(in, in.ReaderIndex())
		if err != nil || !ok {
			return err
		}
		if err := in.SkipBytes(n); err != nil {
			return err
		}
		d.seen = true
		out.Add(PushIDFrame{PushID: pushID})
	}
	if in.ReadableBytes() == 0 {
		return nil
	}
	payload, err := in.Slice(in.ReaderIndex(), in.ReadableBytes())
	if err != nil {
		return err
	}
	if err := in.SkipBytes(in.ReadableBytes()); err != nil {
		payload.Release()
		return err
	}
	out.Add(payload)
	return nil
}

// Write 将 PushIDFrame 编码为裸变长整数，其它消息继续透传。
func (e *PushIDFrameEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(PushIDFrame)
	if !ok {
		return ctx.Write(msg)
	}
	var data []byte
	data, err := appendVarInt(data, frame.PushID)
	if err != nil {
		return err
	}
	return writeBytes(ctx, data)
}

// ChannelActive 在 stream 可写后立即写出 push ID。
func (h *LocalPushIDHandler) ChannelActive(ctx *channel.HandlerContext) {
	if h.wrote {
		ctx.FireChannelActive()
		return
	}
	h.wrote = true
	if err := ctx.Write(PushIDFrame{PushID: h.pushID}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Flush(); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

// ApplyRemotePushStreamPipeline 安装对端 push stream 的 type、push ID 和 frame 解析链。
func ApplyRemotePushStreamPipeline(p *channel.Pipeline, cfg PipelineConfig) error {
	frameDecoder, err := newPipelineFrameDecoder(cfg)
	if err != nil {
		return err
	}
	specs := []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeDecoder, handler: NewStreamTypeDecoder()},
		{name: HandlerNameHTTP3StreamTypeGuard, handler: NewStreamTypeGuard(StreamTypePush)},
		{name: HandlerNameHTTP3PushIDDecoder, handler: NewPushIDDecoder()},
	}
	if cfg.State != nil {
		specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3StateManager, handler: cfg.State})
	}
	specs = append(specs,
		pipelineHandlerSpec{name: HandlerNameHTTP3FrameDecoder, handler: frameDecoder},
		pipelineHandlerSpec{name: HandlerNameHTTP3HeaderDecoder, handler: NewHeaderDecoder(cfg.HeaderCodec)},
		pipelineHandlerSpec{name: HandlerNameHTTP3FrameEncoder, handler: NewEncoder()},
		pipelineHandlerSpec{name: HandlerNameHTTP3HeaderEncoder, handler: NewHeaderEncoder()},
	)
	return addPipelineHandlers(p, specs)
}

// ApplyLocalPushStreamPipeline 安装本端 push stream 的 type、push ID 和 frame 编码链。
func ApplyLocalPushStreamPipeline(p *channel.Pipeline, cfg PipelineConfig, pushID uint64) error {
	writer, err := NewLocalPushIDHandler(pushID)
	if err != nil {
		return err
	}
	specs := []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeEncoder, handler: NewStreamTypeEncoder(StreamTypePush)},
		{name: HandlerNameHTTP3PushIDEncoder, handler: NewPushIDFrameEncoder()},
		{name: HandlerNameHTTP3FrameEncoder, handler: NewEncoder()},
		{name: HandlerNameHTTP3HeaderEncoder, handler: NewHeaderEncoder()},
	}
	if cfg.State != nil {
		specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3StateManager, handler: cfg.State})
	}
	specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3LocalPushID, handler: writer})
	return addPipelineHandlers(p, specs)
}

// RemotePushStreamInitializer 创建对端 push stream 初始化器。
func RemotePushStreamInitializer(cfg PipelineConfig) PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyRemotePushStreamPipeline(ch.Pipeline(), cfg)
	}
}

// LocalPushStreamInitializer 创建本端 push stream 初始化器。
func LocalPushStreamInitializer(cfg PipelineConfig, pushID uint64) PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyLocalPushStreamPipeline(ch.Pipeline(), cfg, pushID)
	}
}
