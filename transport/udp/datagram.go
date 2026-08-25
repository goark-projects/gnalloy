package udp

import "goark.dev/gnalloy/buffer"

// Datagram 是 UDP Pipeline 的入站和出站消息。
type Datagram struct {
	Payload buffer.ByteBuf
	Addr    Address
}

func (d Datagram) Release() {
	if d.Payload != nil {
		d.Payload.Release()
	}
}

func (d Datagram) Valid() bool {
	return d.Payload != nil && d.Addr.Port >= 0 && d.Addr.Port <= 65535 && d.Addr.IP != nil
}
