// Package udp 提供基于原生 UDP socket 的 Datagram Transport。
//
// UDP 没有 accept 阶段，Transport 会把一个或多个 socket 直接注册到
// Worker EventLoop。Pipeline 的原始入站和最终出站消息都是 Datagram。
// readiness 后端使用 recvfrom/sendto，completion 后端使用 datagram IORequest。
//
// 需要编解码时，先用 DatagramToMessageDecoder 把 Datagram.Payload 解码成
// Addressed typed message，再用 MessageToDatagramEncoder 把 Addressed 编码回
// Datagram。Addressed 用来在责任链中保留远端地址，避免业务 Handler 丢失回包目标。
package udp
