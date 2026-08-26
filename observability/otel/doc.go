// Package otel 提供 OpenTelemetry 指标适配器。
//
// 该包是 observability.ChannelRecorder 的外部适配层，核心观测契约仍保持无供应商
// 依赖。生产环境只需传入应用自己的 Meter，即可把 gnalloy Channel 事件写入 OTel。
package otel
