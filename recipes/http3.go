package recipes

import (
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/codec/http3"
)

// HTTP3RequestStream 复用 HTTP/3 request stream 官方 pipeline。
func HTTP3RequestStream(cfg http3.PipelineConfig) bootstrap.ChildInitializer {
	return http3.RequestStreamInitializer(cfg)
}

// HTTP3RemoteControlStream 复用 HTTP/3 remote control stream 官方 pipeline。
func HTTP3RemoteControlStream(cfg http3.PipelineConfig) bootstrap.ChildInitializer {
	return http3.RemoteControlStreamInitializer(cfg)
}

// HTTP3LocalControlStream 复用 HTTP/3 local control stream 官方 pipeline。
func HTTP3LocalControlStream(cfg http3.PipelineConfig) bootstrap.ChildInitializer {
	return http3.LocalControlStreamInitializer(cfg)
}
