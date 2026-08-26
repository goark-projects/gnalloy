package http3

import "goark.dev/gnalloy/channel"

// PipelineInitializer 是 bootstrap Channel 初始化回调的函数签名别名。
type PipelineInitializer = func(ch channel.Channel) error

// RequestStreamInitializer 创建 HTTP/3 bidirectional request stream 初始化器。
func RequestStreamInitializer(cfg PipelineConfig) PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyRequestStreamPipeline(ch.Pipeline(), cfg)
	}
}

// RemoteControlStreamInitializer 创建 HTTP/3 对端 control stream 初始化器。
func RemoteControlStreamInitializer(cfg PipelineConfig) PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyRemoteControlStreamPipeline(ch.Pipeline(), cfg)
	}
}

// LocalControlStreamInitializer 创建 HTTP/3 本端 control stream 初始化器。
func LocalControlStreamInitializer(cfg PipelineConfig) PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyLocalControlStreamPipeline(ch.Pipeline(), cfg)
	}
}

// QPACKEncoderStreamInitializer 创建本端 QPACK encoder stream 初始化器。
func QPACKEncoderStreamInitializer() PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyQPACKEncoderStreamPipeline(ch.Pipeline())
	}
}

// QPACKDecoderStreamInitializer 创建本端 QPACK decoder stream 初始化器。
func QPACKDecoderStreamInitializer() PipelineInitializer {
	return func(ch channel.Channel) error {
		if ch == nil {
			return ErrInvalidPipeline
		}
		return ApplyQPACKDecoderStreamPipeline(ch.Pipeline())
	}
}
