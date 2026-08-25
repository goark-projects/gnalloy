package proxy

import (
	"encoding/binary"
	"net"
	"strconv"
)

const (
	SOCKS5Version byte = 0x05

	SOCKS5MethodNoAuth byte = 0x00

	SOCKS5CommandConnect byte = 0x01

	SOCKS5AddressIPv4   byte = 0x01
	SOCKS5AddressDomain byte = 0x03
	SOCKS5AddressIPv6   byte = 0x04
)

type SOCKS5Reply struct {
	Status byte
	Host   string
	Port   int
}

func AppendSOCKS5Greeting(dst []byte, methods ...byte) ([]byte, error) {
	if len(methods) == 0 || len(methods) > 255 {
		return nil, ErrInvalidMessage
	}
	dst = append(dst, SOCKS5Version, byte(len(methods)))
	return append(dst, methods...), nil
}

func ParseSOCKS5GreetingResponse(data []byte) (byte, int, error) {
	if len(data) < 2 {
		return 0, 0, ErrNeedMore
	}
	if data[0] != SOCKS5Version {
		return 0, 0, ErrInvalidMessage
	}
	return data[1], 2, nil
}

func AppendSOCKS5Connect(dst []byte, address string) ([]byte, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalidMessage
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return nil, ErrInvalidMessage
	}
	dst = append(dst, SOCKS5Version, SOCKS5CommandConnect, 0x00)
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			dst = append(dst, SOCKS5AddressIPv4)
			dst = append(dst, ip4...)
		} else {
			ip16 := ip.To16()
			if ip16 == nil {
				return nil, ErrUnsupportedAddress
			}
			dst = append(dst, SOCKS5AddressIPv6)
			dst = append(dst, ip16...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return nil, ErrUnsupportedAddress
		}
		dst = append(dst, SOCKS5AddressDomain, byte(len(host)))
		dst = append(dst, host...)
	}
	dst = binary.BigEndian.AppendUint16(dst, uint16(port))
	return dst, nil
}

func ParseSOCKS5Reply(data []byte) (SOCKS5Reply, int, error) {
	if len(data) < 4 {
		return SOCKS5Reply{}, 0, ErrNeedMore
	}
	if data[0] != SOCKS5Version || data[2] != 0x00 {
		return SOCKS5Reply{}, 0, ErrInvalidMessage
	}
	host, idx, err := parseSOCKS5Address(data, 3)
	if err != nil {
		return SOCKS5Reply{}, 0, err
	}
	if len(data)-idx < 2 {
		return SOCKS5Reply{}, 0, ErrNeedMore
	}
	port := int(binary.BigEndian.Uint16(data[idx : idx+2]))
	return SOCKS5Reply{Status: data[1], Host: host, Port: port}, idx + 2, nil
}

func parseSOCKS5Address(data []byte, idx int) (string, int, error) {
	if idx >= len(data) {
		return "", 0, ErrNeedMore
	}
	switch data[idx] {
	case SOCKS5AddressIPv4:
		if len(data)-idx < 5 {
			return "", 0, ErrNeedMore
		}
		return net.IPv4(data[idx+1], data[idx+2], data[idx+3], data[idx+4]).String(), idx + 5, nil
	case SOCKS5AddressIPv6:
		if len(data)-idx < 17 {
			return "", 0, ErrNeedMore
		}
		ip := make(net.IP, net.IPv6len)
		copy(ip, data[idx+1:idx+17])
		return ip.String(), idx + 17, nil
	case SOCKS5AddressDomain:
		if len(data)-idx < 2 {
			return "", 0, ErrNeedMore
		}
		n := int(data[idx+1])
		if len(data)-idx < 2+n {
			return "", 0, ErrNeedMore
		}
		return string(data[idx+2 : idx+2+n]), idx + 2 + n, nil
	default:
		return "", 0, ErrUnsupportedAddress
	}
}
