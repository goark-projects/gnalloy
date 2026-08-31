// Package quic 提供 Gnalloy QUIC 的稳定门面。
//
// 本包不实现 QUIC packet、frame、ACK、丢包恢复、拥塞控制或连接迁移协议栈。
// 这些高风险协议语义统一委托给 quic-go，Gnalloy 只在这里暴露符合自身
// Bootstrap、Channel 和 provider 约定的 Go 化 API。
package quic
