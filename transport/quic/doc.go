// Package quic 提供 Gnalloy QUIC 的生产级连接适配层。
//
// 本包不重复实现 TLS 1.3 packet protection、密钥派生、重传和拥塞控制等高风险
// 协议细节，而是把完整协议语义委托给成熟 QUIC 栈。Gnalloy 在这里提供稳定、
// 小而明确的 Go 接口，供业务 stream、HTTP/3 control/QPACK stream 和外部互通
// 测试使用。
package quic
