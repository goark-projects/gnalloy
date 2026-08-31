package http3

import "goark.dev/gnalloy/channel"

// ApplyRequestStreamPipeline 安装 HTTP/3 bidirectional request stream 的编解码链。
func ApplyRequestStreamPipeline(p *channel.Pipeline, cfg PipelineConfig) error {
	frameDecoder, err := newPipelineFrameDecoder(cfg)
	if err != nil {
		return err
	}
	specs := []pipelineHandlerSpec{
		{name: HandlerNameHTTP3FrameDecoder, handler: frameDecoder},
		{name: HandlerNameHTTP3HeaderDecoder, handler: NewHeaderDecoder(cfg.HeaderCodec)},
		{name: HandlerNameHTTP3FrameEncoder, handler: NewEncoder()},
		{name: HandlerNameHTTP3HeaderEncoder, handler: NewHeaderEncoder()},
	}
	if cfg.State != nil {
		specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3StateManager, handler: cfg.State})
	}
	return addPipelineHandlers(p, specs)
}
