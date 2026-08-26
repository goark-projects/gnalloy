// Package quic 定义 gnalloy QUIC transport 的协议边界。
//
// QUIC 不是普通 UDP socket 的别名。完整实现必须覆盖 TLS 1.3 握手、包号空间、
// ACK、丢包恢复、拥塞控制、连接迁移、流控和 stream 复用。本包当前提供 Go 化
// 包引擎：负责 UDP 绑定、包编解码、连接 ID 路由和 frame 分发边界。
// 其 readiness/completion 后端能力继承自 UDP transport。
package quic
