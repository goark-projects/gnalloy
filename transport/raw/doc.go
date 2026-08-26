// Package raw 提供基于原生 raw socket 的 IP Packet Transport。
//
// raw transport 面向 ICMP、ICMPv6、自定义 IP 协议以及需要直接处理 IP 包的场景。
// Pipeline 的原始入站和最终出站消息都是 Packet。上层 codec 可通过
// PacketToMessageDecoder 和 MessageToPacketEncoder 把 Packet 转成业务消息。
// 如果业务消息已经编码为 ByteBuf，可用 PacketEncoder 直接把 Addressed 写成 Packet。
//
// raw socket 通常需要管理员或 CAP_NET_RAW 权限；不同操作系统对 raw TCP/UDP
// 发包限制不同，具体错误会从底层系统调用原样返回。
// completion 后端复用 datagram IORequest，但运行成功仍取决于平台 raw socket 权限。
package raw
