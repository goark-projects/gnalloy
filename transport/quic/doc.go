// Package quic 定义 gnalloy QUIC transport 的协议边界。
//
// QUIC 不是普通 UDP socket 的别名。完整实现必须覆盖 TLS 1.3 握手、包号空间、
// ACK、丢包恢复、拥塞控制、连接迁移、流控和 stream 复用。本包先固定 Go 化 API
// 和基础数据结构，避免后续实现时把 UDP Datagram 语义泄漏给上层业务。
package quic
