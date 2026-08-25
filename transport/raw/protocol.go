package raw

const (
	ProtocolICMP   = 1
	ProtocolTCP    = 6
	ProtocolUDP    = 17
	ProtocolIPv6   = 41
	ProtocolICMPv6 = 58
	ProtocolRaw    = 255
)

// Family 描述 raw socket 使用的 IP 地址族。
type Family uint8

const (
	FamilyIPv4 Family = iota + 1
	FamilyIPv6
)
