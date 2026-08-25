// Package ip 提供 IPv4/IPv6 header 编解码和 raw transport 责任链适配。
//
// ProtocolCodec 是自定义 IP 协议的双工入口：PacketFormatPayload 适合内核补
// IP 头的 raw socket，PacketFormatIP 适合应用自己构造完整 IPv4/IPv6 packet。
// 两种模式在上层都使用 raw.Addressed 承载远端地址、协议号和业务消息。
package ip
