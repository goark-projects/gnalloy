package ip

import (
	"net"

	"goark.dev/gnalloy/buffer"
)

const (
	Version4 = 4
	Version6 = 6

	DefaultTTL      = 64
	DefaultHopLimit = 64

	ipv4HeaderLength = 20
	ipv6HeaderLength = 40
)

// 常用 IP 协议号，保持和 IANA/RFC 定义一致。
const (
	ProtocolICMP   = 1
	ProtocolTCP    = 6
	ProtocolUDP    = 17
	ProtocolIPv6   = 41
	ProtocolICMPv6 = 58
	ProtocolRaw    = 255
)

// Header 表示 IPv4/IPv6 公共头部。未适用字段保持零值。
type Header struct {
	Version      int
	HeaderLength int
	TotalLength  int

	TrafficClass  int
	FlowLabel     uint32
	PayloadLength int

	Identification uint16
	Flags          uint8
	FragmentOffset uint16
	TTL            uint8
	HopLimit       uint8

	Protocol   int
	NextHeader int

	Checksum uint16

	Source      net.IP
	Destination net.IP
}

func (h Header) PayloadProtocol() int {
	if h.Version == Version6 {
		return h.NextHeader
	}
	return h.Protocol
}

func (h Header) payloadLength() int {
	switch h.Version {
	case Version4:
		return h.TotalLength - h.HeaderLength
	case Version6:
		return h.PayloadLength
	default:
		return -1
	}
}

// Packet 是 IP header 与 payload 的零拷贝组合。
type Packet struct {
	Header  Header
	Payload buffer.ByteBuf
}

func (p Packet) Release() {
	if p.Payload != nil {
		p.Payload.Release()
	}
}

func (p Packet) Valid() bool {
	return p.Header.Version == Version4 || p.Header.Version == Version6
}
