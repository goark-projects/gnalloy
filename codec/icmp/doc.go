// Package icmp 提供 ICMP/ICMPv6 编解码器。
//
// 入站通常接在 raw.PacketToMessageDecoder 之后，消费 raw.Addressed{Message:
// buffer.ByteBuf}，输出 raw.Addressed{Message: *icmp.Message}。出站方向消费
// raw.Addressed{Message: *icmp.Message}，编码成 raw.Addressed{Message: ByteBuf}，
// 再交给 raw.MessageToPacketEncoder 写回 raw socket。
package icmp
