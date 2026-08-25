package quic

import "goark.dev/gnalloy/buffer"

type PacketType uint8

const (
	PacketInitial PacketType = iota + 1
	PacketZeroRTT
	PacketHandshake
	PacketRetry
	PacketShort
)

// Packet 是 QUIC 协议引擎内部传递的 UDP payload 视图。
type Packet struct {
	Header Header

	Type          PacketType
	Version       Version
	DestinationID ConnectionID
	SourceID      ConnectionID

	PacketNumberLength int
	PacketNumber       uint64

	Payload buffer.ByteBuf
}

func (p Packet) Release() {
	if p.Payload != nil {
		p.Payload.Release()
	}
}

func (p Packet) Valid() bool {
	if !validPacketType(p.Type) {
		return false
	}
	if p.Type != PacketShort && !p.Version.Valid() {
		return false
	}
	return true
}

func validPacketType(packetType PacketType) bool {
	return packetType >= PacketInitial && packetType <= PacketShort
}
