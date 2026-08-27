// Package protocol 提供跨传输层的应用协议装配 API。
//
// 本包只处理上层 request-response 交互，不接管 TCP、UDP、raw、L2 或 QUIC 的
// 底层 socket 生命周期。客户端通过 bootstrap.ClientTransport 注入具体传输，
// 服务端通过 bootstrap.ServerTransport 绑定具体传输，再选择 stream、datagram、
// packet 或 frame 适配器完成消息映射。
package protocol
