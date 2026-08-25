package icmp

import "net"

// Checksum 计算 ICMP 使用的一补和校验值。
func Checksum(data []byte) uint16 {
	sum := uint32(0)
	for len(data) >= 2 {
		sum += uint32(data[0])<<8 | uint32(data[1])
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// ChecksumIPv6 使用 IPv6 伪头部计算 ICMPv6 校验值。
func ChecksumIPv6(src net.IP, dst net.IP, payload []byte) (uint16, error) {
	src16 := src.To16()
	dst16 := dst.To16()
	if src16 == nil || dst16 == nil || src.To4() != nil || dst.To4() != nil {
		return 0, ErrMissingIPv6PseudoHdr
	}
	sum := uint32(0)
	sum = addWords(sum, src16)
	sum = addWords(sum, dst16)
	length := uint32(len(payload))
	sum += (length >> 16) & 0xffff
	sum += length & 0xffff
	sum += uint32(58)
	sum = addWords(sum, payload)
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum), nil
}

func addWords(sum uint32, data []byte) uint32 {
	for len(data) >= 2 {
		sum += uint32(data[0])<<8 | uint32(data[1])
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	return sum
}
