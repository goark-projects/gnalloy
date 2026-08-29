package haproxy

import (
	"bytes"
	"net"
	"net/netip"

	"goark.dev/gnalloy/buffer"
)

func ipv4String(in *buffer.CompositeByteBuf, start int) string {
	var ip [4]byte
	for i := range ip {
		ip[i], _ = in.GetByte(start + i)
	}
	return netip.AddrFrom4(ip).String()
}

func readUint16(in *buffer.CompositeByteBuf, start int) (uint16, error) {
	hi, hiOK := in.GetByte(start)
	lo, loOK := in.GetByte(start + 1)
	if !hiOK || !loOK {
		return 0, buffer.ErrInvalidIndex
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

func ipString(in *buffer.CompositeByteBuf, start int, length int) string {
	ip := make(net.IP, length)
	for i := range ip {
		ip[i], _ = in.GetByte(start + i)
	}
	return ip.String()
}

func unixAddressString(in *buffer.CompositeByteBuf, start int) string {
	value := make([]byte, 108)
	for i := range value {
		value[i], _ = in.GetByte(start + i)
	}
	return string(bytes.TrimRight(value, "\x00"))
}
