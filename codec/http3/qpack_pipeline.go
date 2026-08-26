package http3

import "goark.dev/gnalloy/channel"

// ApplyQPACKEncoderStreamPipeline 安装本端 QPACK encoder stream 的 stream type 前缀写入器。
func ApplyQPACKEncoderStreamPipeline(p *channel.Pipeline) error {
	return ApplyLocalQPACKEncoderStreamPipeline(p)
}

// ApplyQPACKDecoderStreamPipeline 安装本端 QPACK decoder stream 的 stream type 前缀写入器。
func ApplyQPACKDecoderStreamPipeline(p *channel.Pipeline) error {
	return ApplyLocalQPACKDecoderStreamPipeline(p)
}

// ApplyLocalQPACKEncoderStreamPipeline 安装本端 QPACK encoder stream 的 stream type 前缀写入器。
func ApplyLocalQPACKEncoderStreamPipeline(p *channel.Pipeline) error {
	return addPipelineHandlers(p, []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeEncoder, handler: NewStreamTypeEncoder(StreamTypeQPACKEncoder)},
	})
}

// ApplyLocalQPACKDecoderStreamPipeline 安装本端 QPACK decoder stream 的 stream type 前缀写入器。
func ApplyLocalQPACKDecoderStreamPipeline(p *channel.Pipeline) error {
	return addPipelineHandlers(p, []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeEncoder, handler: NewStreamTypeEncoder(StreamTypeQPACKDecoder)},
	})
}

// ApplyRemoteQPACKEncoderStreamPipeline 安装对端 QPACK encoder stream 的 stream type 解析与校验链。
func ApplyRemoteQPACKEncoderStreamPipeline(p *channel.Pipeline) error {
	return addPipelineHandlers(p, []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeDecoder, handler: NewStreamTypeDecoder()},
		{name: HandlerNameHTTP3StreamTypeGuard, handler: NewStreamTypeGuard(StreamTypeQPACKEncoder)},
	})
}

// ApplyRemoteQPACKDecoderStreamPipeline 安装对端 QPACK decoder stream 的 stream type 解析与校验链。
func ApplyRemoteQPACKDecoderStreamPipeline(p *channel.Pipeline) error {
	return addPipelineHandlers(p, []pipelineHandlerSpec{
		{name: HandlerNameHTTP3StreamTypeDecoder, handler: NewStreamTypeDecoder()},
		{name: HandlerNameHTTP3StreamTypeGuard, handler: NewStreamTypeGuard(StreamTypeQPACKDecoder)},
	})
}
