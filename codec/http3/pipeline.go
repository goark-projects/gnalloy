package http3

import "goark.dev/gnalloy/channel"

const (
	// HandlerNameHTTP3FrameDecoder 是 HTTP/3 frame 入站解码器的默认名称。
	HandlerNameHTTP3FrameDecoder = "http3-frame-decoder"
	// HandlerNameHTTP3HeaderDecoder 是 HTTP/3 QPACK header 入站解码器的默认名称。
	HandlerNameHTTP3HeaderDecoder = "http3-header-decoder"
	// HandlerNameHTTP3FrameEncoder 是 HTTP/3 frame 出站编码器的默认名称。
	HandlerNameHTTP3FrameEncoder = "http3-frame-encoder"
	// HandlerNameHTTP3HeaderEncoder 是 HTTP/3 QPACK header 出站编码器的默认名称。
	HandlerNameHTTP3HeaderEncoder = "http3-header-encoder"
	// HandlerNameHTTP3StreamTypeDecoder 是 HTTP/3 单向 stream type 入站解码器的默认名称。
	HandlerNameHTTP3StreamTypeDecoder = "http3-stream-type-decoder"
	// HandlerNameHTTP3StreamTypeEncoder 是 HTTP/3 单向 stream type 出站编码器的默认名称。
	HandlerNameHTTP3StreamTypeEncoder = "http3-stream-type-encoder"
	// HandlerNameHTTP3StreamTypeGuard 是 HTTP/3 单向 stream type 校验器的默认名称。
	HandlerNameHTTP3StreamTypeGuard = "http3-stream-type-guard"
	// HandlerNameHTTP3ControlStream 是 HTTP/3 control stream 入站校验器的默认名称。
	HandlerNameHTTP3ControlStream = "http3-control-stream"
	// HandlerNameHTTP3LocalControlStream 是 HTTP/3 control stream 本端 SETTINGS 写入器的默认名称。
	HandlerNameHTTP3LocalControlStream = "http3-local-control-stream"
	// HandlerNameHTTP3PushIDDecoder 是 HTTP/3 push stream push ID 入站解码器的默认名称。
	HandlerNameHTTP3PushIDDecoder = "http3-push-id-decoder"
	// HandlerNameHTTP3PushIDEncoder 是 HTTP/3 push stream push ID 出站编码器的默认名称。
	HandlerNameHTTP3PushIDEncoder = "http3-push-id-encoder"
	// HandlerNameHTTP3LocalPushID 是 HTTP/3 本端 push stream push ID 写入器的默认名称。
	HandlerNameHTTP3LocalPushID = "http3-local-push-id"
	// HandlerNameHTTP3StateManager 是 HTTP/3 连接级状态管理器的默认名称。
	HandlerNameHTTP3StateManager = "http3-state-manager"
)

// PipelineConfig 描述 HTTP/3 stream pipeline 的装配参数。
type PipelineConfig struct {
	// MaxFrameSize 限制单个 HTTP/3 frame 的 payload 字节数，0 使用协议默认值。
	MaxFrameSize int
	// MaxSettings 限制 SETTINGS frame 中的配置项数量，0 使用默认保护阈值。
	MaxSettings int
	// HeaderCodec 描述 QPACK header 编解码边界。
	HeaderCodec HeaderCodecConfig
	// Settings 是本端 control stream 激活时主动发送的 SETTINGS。
	Settings []Setting
	// State 是连接级 HTTP/3 状态管理器；为空时仅安装流内编解码链。
	State *StateManager
}

type pipelineHandlerSpec struct {
	name    string
	handler channel.Handler
}

func newPipelineFrameDecoder(cfg PipelineConfig) (*Decoder, error) {
	decoder, err := NewDecoder(cfg.MaxFrameSize)
	if err != nil {
		return nil, err
	}
	if cfg.MaxSettings > 0 {
		if err := decoder.SetMaxSettings(cfg.MaxSettings); err != nil {
			return nil, err
		}
	}
	return decoder, nil
}

func addPipelineHandlers(p *channel.Pipeline, specs []pipelineHandlerSpec) error {
	if p == nil || len(specs) == 0 {
		return ErrInvalidPipeline
	}
	for _, spec := range specs {
		if spec.name == "" || spec.handler == nil {
			return ErrInvalidPipeline
		}
		if _, exists := p.Context(spec.name); exists {
			return channel.ErrDuplicateHandler
		}
	}

	added := make([]string, 0, len(specs))
	for _, spec := range specs {
		if err := p.AddLast(spec.name, spec.handler); err != nil {
			removePipelineHandlers(p, added)
			return err
		}
		added = append(added, spec.name)
	}
	return nil
}

func removePipelineHandlers(p *channel.Pipeline, names []string) {
	for i := len(names) - 1; i >= 0; i-- {
		_ = p.Remove(names[i])
	}
}
