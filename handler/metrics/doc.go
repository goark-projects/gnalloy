// Package metrics 提供 Pipeline 级 Channel 指标采集 handler。
//
// Handler 只负责把 Channel 事件映射到 observability.ChannelRecorder，不保存
// 高基数字段，也不绑定任何外部指标系统。
package metrics
