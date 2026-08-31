package http3

import "goark.dev/gnalloy/channel"

// ApplyRemoteControlStreamPipeline 安装对端 HTTP/3 control stream 的解析与顺序校验链。
func ApplyRemoteControlStreamPipeline(p *channel.Pipeline, cfg PipelineConfig) error {
	frameDecoder, err := newPipelineFrameDecoder(cfg)
	if err != nil {
		return err
	}
	specs := []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeDecoder, handler: NewStreamTypeDecoder()},
		{name: HandlerNameHTTP3FrameDecoder, handler: frameDecoder},
		{name: HandlerNameHTTP3ControlStream, handler: NewControlStreamHandler()},
	}
	if cfg.State != nil {
		specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3StateManager, handler: cfg.State})
	}
	return addPipelineHandlers(p, specs)
}

// ApplyLocalControlStreamPipeline 安装本端 HTTP/3 control stream 的 type 前缀、frame 编码和 SETTINGS 写入器。
func ApplyLocalControlStreamPipeline(p *channel.Pipeline, cfg PipelineConfig) error {
	writer, err := NewLocalControlStreamHandler(cfg.Settings)
	if err != nil {
		return err
	}
	specs := []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeEncoder, handler: NewStreamTypeEncoder(StreamTypeControl)},
		{name: HandlerNameHTTP3FrameEncoder, handler: NewEncoder()},
	}
	if cfg.State != nil {
		specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3StateManager, handler: cfg.State})
	}
	specs = append(specs, pipelineHandlerSpec{name: HandlerNameHTTP3LocalControlStream, handler: writer})
	return addPipelineHandlers(p, specs)
}
