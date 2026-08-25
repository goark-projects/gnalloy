package raw

import "goark.dev/gnalloy/buffer"

// Packet 是 raw Pipeline 的入站和出站消息。
type Packet struct {
	Payload  buffer.ByteBuf
	Addr     Address
	Protocol int
}

func (p Packet) Release() {
	if p.Payload != nil {
		p.Payload.Release()
	}
}

func (p Packet) Valid() bool {
	return p.Payload != nil && p.Addr.IP != nil && validProtocol(p.Protocol)
}

func validProtocol(protocol int) bool {
	return protocol > 0 && protocol <= 255
}
