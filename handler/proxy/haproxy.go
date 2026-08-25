package proxy

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
)

var haproxyV2Signature = []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 'Q', 'U', 'I', 'T', 0x0a}

type HAProxyInfo struct {
	Version    int
	Command    string
	Protocol   string
	SourceIP   net.IP
	DestIP     net.IP
	SourcePort int
	DestPort   int
}

func ParseHAProxyHeader(data []byte) (HAProxyInfo, int, error) {
	if bytes.HasPrefix(data, haproxyV2Signature) {
		return parseHAProxyV2(data)
	}
	if bytes.HasPrefix(data, []byte("PROXY ")) {
		return parseHAProxyV1(data)
	}
	return HAProxyInfo{}, 0, ErrInvalidMessage
}

func parseHAProxyV1(data []byte) (HAProxyInfo, int, error) {
	end := bytes.Index(data, []byte("\r\n"))
	if end < 0 {
		return HAProxyInfo{}, 0, ErrNeedMore
	}
	fields := strings.Fields(string(data[:end]))
	if len(fields) < 2 || fields[0] != "PROXY" {
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	info := HAProxyInfo{Version: 1, Command: "PROXY", Protocol: fields[1]}
	if fields[1] == "UNKNOWN" {
		return info, end + 2, nil
	}
	if len(fields) != 6 {
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	src := net.ParseIP(fields[2])
	dst := net.ParseIP(fields[3])
	srcPort, err := strconv.Atoi(fields[4])
	if err != nil {
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	dstPort, err := strconv.Atoi(fields[5])
	if err != nil {
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	if src == nil || dst == nil || srcPort < 0 || srcPort > 65535 || dstPort < 0 || dstPort > 65535 {
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	info.SourceIP = src
	info.DestIP = dst
	info.SourcePort = srcPort
	info.DestPort = dstPort
	return info, end + 2, nil
}

func parseHAProxyV2(data []byte) (HAProxyInfo, int, error) {
	if len(data) < 16 {
		return HAProxyInfo{}, 0, ErrNeedMore
	}
	versionCommand := data[12]
	if versionCommand>>4 != 0x02 {
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	length := int(binary.BigEndian.Uint16(data[14:16]))
	if len(data) < 16+length {
		return HAProxyInfo{}, 0, ErrNeedMore
	}
	info := HAProxyInfo{Version: 2}
	switch versionCommand & 0x0f {
	case 0x00:
		info.Command = "LOCAL"
	case 0x01:
		info.Command = "PROXY"
	default:
		return HAProxyInfo{}, 0, ErrInvalidMessage
	}
	payload := data[16 : 16+length]
	switch data[13] {
	case 0x11:
		if len(payload) < 12 {
			return HAProxyInfo{}, 0, ErrInvalidMessage
		}
		info.Protocol = "TCP4"
		info.SourceIP = net.IPv4(payload[0], payload[1], payload[2], payload[3])
		info.DestIP = net.IPv4(payload[4], payload[5], payload[6], payload[7])
		info.SourcePort = int(binary.BigEndian.Uint16(payload[8:10]))
		info.DestPort = int(binary.BigEndian.Uint16(payload[10:12]))
	case 0x21:
		if len(payload) < 36 {
			return HAProxyInfo{}, 0, ErrInvalidMessage
		}
		info.Protocol = "TCP6"
		info.SourceIP = append(net.IP(nil), payload[0:16]...)
		info.DestIP = append(net.IP(nil), payload[16:32]...)
		info.SourcePort = int(binary.BigEndian.Uint16(payload[32:34]))
		info.DestPort = int(binary.BigEndian.Uint16(payload[34:36]))
	case 0x00:
		info.Protocol = "UNKNOWN"
	default:
		return HAProxyInfo{}, 0, ErrUnsupportedProtocol
	}
	return info, 16 + length, nil
}
