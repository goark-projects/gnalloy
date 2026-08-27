package recipes

import (
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/mqtt"
)

const (
	// HandlerNameMQTTFrameDecoder 是 MQTT frame 边界解码器的默认名称。
	HandlerNameMQTTFrameDecoder = "mqtt-frame-decoder"
	// HandlerNameMQTTTypedDecoder 是 MQTT typed frame 解码器的默认名称。
	HandlerNameMQTTTypedDecoder = "mqtt-typed-decoder"
	// HandlerNameMQTTFramePrepender 是 MQTT frame 出站编码器的默认名称。
	HandlerNameMQTTFramePrepender = "mqtt-frame-prepender"
)

// MQTTConfig 描述 MQTT frame pipeline。
type MQTTConfig struct {
	MaxFrameLength int
	Typed          bool
}

// MQTTFrames 创建 MQTT frame pipeline。
func MQTTFrames(cfg MQTTConfig, app ...HandlerSpec) bootstrap.ChildInitializer {
	cfg = normalizeMQTTConfig(cfg)
	base := []HandlerSpec{
		UseFactory(HandlerNameMQTTFrameDecoder, func() (channel.Handler, error) {
			return mqtt.NewFrameDecoder(cfg.MaxFrameLength)
		}),
		UseFactory(HandlerNameMQTTFramePrepender, func() (channel.Handler, error) {
			return mqtt.NewFramePrepender(), nil
		}),
	}
	if cfg.Typed {
		base = append(base, UseFactory(HandlerNameMQTTTypedDecoder, func() (channel.Handler, error) {
			return mqtt.NewTypedFrameDecoder(), nil
		}))
	}
	return Initializer(appendSpecs(base, app)...)
}

func normalizeMQTTConfig(cfg MQTTConfig) MQTTConfig {
	if cfg.MaxFrameLength <= 0 {
		cfg.MaxFrameLength = 256 * 1024
	}
	return cfg
}
