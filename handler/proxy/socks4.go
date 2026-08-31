package proxy

import (
	"encoding/binary"
	"net"
	"strconv"
)

const (
	SOCKS4Version byte = 0x04

	SOCKS4CommandConnect byte = 0x01
	SOCKS4CommandBind    byte = 0x02

	SOCKS4StatusGranted        byte = 0x5a
	SOCKS4StatusRejected       byte = 0x5b
	SOCKS4StatusIdentdFailed   byte = 0x5c
	SOCKS4StatusIdentdMismatch byte = 0x5d
)

// SOCKS4Reply 表示 SOCKS4/SOCKS4a 代理返回。
type SOCKS4Reply struct {
	Status  byte
	Address string
}

// AppendSOCKS4Connect 追加 SOCKS4 CONNECT 请求。
func AppendSOCKS4Connect(dst []byte, address string, userID string) ([]byte, error) {
	return AppendSOCKS4Command(dst, SOCKS4CommandConnect, address, userID)
}

// AppendSOCKS4Bind 追加 SOCKS4 BIND 请求。
func AppendSOCKS4Bind(dst []byte, address string, userID string) ([]byte, error) {
	return AppendSOCKS4Command(dst, SOCKS4CommandBind, address, userID)
}

// AppendSOCKS4Command 追加 SOCKS4/SOCKS4a 命令请求。
func AppendSOCKS4Command(dst []byte, command byte, address string, userID string) ([]byte, error) {
	if command != SOCKS4CommandConnect && command != SOCKS4CommandBind {
		return nil, ErrInvalidMessage
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalidMessage
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return nil, ErrInvalidMessage
	}
	dst = append(dst, SOCKS4Version, command)
	dst = binary.BigEndian.AppendUint16(dst, uint16(port))
	if ip := net.ParseIP(host).To4(); ip != nil {
		dst = append(dst, ip...)
	} else {
		if host == "" {
			return nil, ErrUnsupportedAddress
		}
		dst = append(dst, 0, 0, 0, 1)
	}
	dst = append(dst, userID...)
	dst = append(dst, 0)
	if net.ParseIP(host).To4() == nil {
		dst = append(dst, host...)
		dst = append(dst, 0)
	}
	return dst, nil
}

// ParseSOCKS4Reply 解析 SOCKS4 固定 8 字节响应。
func ParseSOCKS4Reply(data []byte) (SOCKS4Reply, int, error) {
	if len(data) < 8 {
		return SOCKS4Reply{}, 0, ErrNeedMore
	}
	if data[0] != 0 {
		return SOCKS4Reply{}, 0, ErrInvalidMessage
	}
	port := int(binary.BigEndian.Uint16(data[2:4]))
	ip := net.IPv4(data[4], data[5], data[6], data[7]).String()
	return SOCKS4Reply{
		Status:  data[1],
		Address: net.JoinHostPort(ip, strconv.Itoa(port)),
	}, 8, nil
}
