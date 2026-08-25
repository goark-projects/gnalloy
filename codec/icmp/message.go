package icmp

import (
	"net"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/raw"
)

const (
	TypeEchoReply       uint8 = 0
	TypeEchoRequest     uint8 = 8
	TypeIPv6EchoRequest uint8 = 128
	TypeIPv6EchoReply   uint8 = 129
	echoHeaderLength          = 8
	messageHeaderLength       = 4
)

// Message 表示一个 ICMP/ICMPv6 消息。Payload 是零拷贝视图，使用后必须释放。
type Message struct {
	Type       uint8
	Code       uint8
	Checksum   uint16
	Identifier uint16
	Sequence   uint16
	Protocol   int
	SourceIP   net.IP
	Payload    buffer.ByteBuf
}

func NewEchoRequest(protocol int, identifier uint16, sequence uint16, payload buffer.ByteBuf) *Message {
	t := TypeEchoRequest
	if protocol == raw.ProtocolICMPv6 {
		t = TypeIPv6EchoRequest
	}
	return &Message{Type: t, Identifier: identifier, Sequence: sequence, Protocol: protocol, Payload: payload}
}

func NewEchoReply(protocol int, identifier uint16, sequence uint16, payload buffer.ByteBuf) *Message {
	t := TypeEchoReply
	if protocol == raw.ProtocolICMPv6 {
		t = TypeIPv6EchoReply
	}
	return &Message{Type: t, Identifier: identifier, Sequence: sequence, Protocol: protocol, Payload: payload}
}

func (m *Message) Release() {
	if m != nil && m.Payload != nil {
		m.Payload.Release()
		m.Payload = nil
	}
}

func (m *Message) IsEchoRequest() bool {
	return m != nil && (m.Type == TypeEchoRequest || m.Type == TypeIPv6EchoRequest)
}

func (m *Message) IsEchoReply() bool {
	return m != nil && (m.Type == TypeEchoReply || m.Type == TypeIPv6EchoReply)
}

func (m *Message) IsEcho() bool {
	return m.IsEchoRequest() || m.IsEchoReply()
}

func (m *Message) encodedLength() int {
	if m == nil {
		return 0
	}
	n := messageHeaderLength
	if m.IsEcho() {
		n = echoHeaderLength
	}
	if m.Payload != nil {
		n += m.Payload.ReadableBytes()
	}
	return n
}

func protocolForType(t uint8) int {
	switch t {
	case TypeIPv6EchoRequest, TypeIPv6EchoReply:
		return raw.ProtocolICMPv6
	default:
		return raw.ProtocolICMP
	}
}
