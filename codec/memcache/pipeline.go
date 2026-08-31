package memcache

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	// DefaultFrameCodecName 是 memcache wire codec 的默认 pipeline 名称。
	DefaultFrameCodecName = "memcache-frame-codec"
	// DefaultClientCodecName 是 memcache client 对象 codec 的默认 pipeline 名称。
	DefaultClientCodecName = "memcache-client-codec"
	// DefaultServerCodecName 是 memcache server 对象 codec 的默认 pipeline 名称。
	DefaultServerCodecName = "memcache-server-codec"
)

// ClientCodec 解码 response frame，并把 request 对象编码为 frame。
type ClientCodec struct {
	*codec.MessageToMessageCodec
}

// NewClientCodec 创建 Memcached binary client 侧对象 codec。
func NewClientCodec() *ClientCodec {
	return &ClientCodec{MessageToMessageCodec: codec.NewMessageToMessageCodec(clientInboundDecoder{}, clientOutboundEncoder{})}
}

// ServerCodec 解码 request frame，并把 response 对象编码为 frame。
type ServerCodec struct {
	*codec.MessageToMessageCodec
}

// NewServerCodec 创建 Memcached binary server 侧对象 codec。
func NewServerCodec() *ServerCodec {
	return &ServerCodec{MessageToMessageCodec: codec.NewMessageToMessageCodec(serverInboundDecoder{}, serverOutboundEncoder{})}
}

// NewFrameCodec 创建 binary frame 双工 codec。
func NewFrameCodec(maxFrameLength int) (channel.Handler, error) {
	decoder, err := NewFrameDecoder(maxFrameLength)
	if err != nil {
		return nil, err
	}
	return codec.NewCombinedChannelDuplexHandler(decoder, NewFrameEncoder()), nil
}

// AddClientCodec 按 wire codec -> client object codec 顺序安装 pipeline。
func AddClientCodec(pipeline *channel.Pipeline, maxFrameLength int) error {
	return AddNamedClientCodec(pipeline, DefaultFrameCodecName, DefaultClientCodecName, maxFrameLength)
}

// AddNamedClientCodec 使用显式名称安装 client 侧 codec。
func AddNamedClientCodec(pipeline *channel.Pipeline, frameName string, objectName string, maxFrameLength int) error {
	return addNamedCodec(pipeline, frameName, objectName, maxFrameLength, NewClientCodec())
}

// AddServerCodec 按 wire codec -> server object codec 顺序安装 pipeline。
func AddServerCodec(pipeline *channel.Pipeline, maxFrameLength int) error {
	return AddNamedServerCodec(pipeline, DefaultFrameCodecName, DefaultServerCodecName, maxFrameLength)
}

// AddNamedServerCodec 使用显式名称安装 server 侧 codec。
func AddNamedServerCodec(pipeline *channel.Pipeline, frameName string, objectName string, maxFrameLength int) error {
	return addNamedCodec(pipeline, frameName, objectName, maxFrameLength, NewServerCodec())
}

func addNamedCodec(pipeline *channel.Pipeline, frameName string, objectName string, maxFrameLength int, objectCodec channel.Handler) error {
	if pipeline == nil || frameName == "" || objectName == "" {
		return ErrInvalidConfig
	}
	frameCodec, err := NewFrameCodec(maxFrameLength)
	if err != nil {
		return err
	}
	if err := pipeline.AddLast(frameName, frameCodec); err != nil {
		return err
	}
	if err := pipeline.AddLast(objectName, objectCodec); err != nil {
		_ = pipeline.Remove(frameName)
		return err
	}
	return nil
}
