// Package observability 提供 gnalloy 的轻量观测契约。
//
// 该包只定义低基数、无供应商绑定的指标接口和本地原子聚合器。生产环境可
// 通过实现 ChannelRecorder 接入 Prometheus、OpenTelemetry 或其他 APM，
// 热路径默认实现不分配、不记录高基数字段。
package observability
