package application

import (
	"context"
	"time"

	"goark.dev/gnalloy/transport/quic"
)

// Stream 是应用协议装配使用的 QUIC 双向 stream 最小契约。
type Stream = quic.Stream

// StreamCodec 定义 QUIC stream 上的 request-response 编解码。
type StreamCodec interface {
	WriteRequest(stream Stream, payload []byte) error
	ReadResponse(stream Stream) ([]byte, error)
}

// StreamExchanger 在每次请求中建立连接、打开双向 stream、写请求并读取响应。
type StreamExchanger struct {
	Dialer  quic.Dialer
	Config  quic.Config
	Codec   StreamCodec
	Timeout time.Duration
}

// Exchange 执行一次 QUIC stream request-response 交换。
func (e StreamExchanger) Exchange(ctx context.Context, address string, payload []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}
	dialer := e.Dialer
	if dialer == nil {
		dialer = quic.DefaultDialer{}
	}
	codec := e.Codec
	if codec == nil {
		codec = LengthPrefixedCodec{}
	}
	conn, err := dialer.DialAddr(ctx, address, e.Config)
	if err != nil {
		return nil, err
	}
	defer conn.CloseWithError(0, "application exchange complete")
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetDeadline(deadline)
	}
	if err := codec.WriteRequest(stream, payload); err != nil {
		stream.CancelWrite(0)
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}
	return codec.ReadResponse(stream)
}
