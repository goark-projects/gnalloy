package tcp

import (
	"net"
	"strconv"
)

type parsedAddress struct {
	host string
	port int
	ip   net.IP
	ipv6 bool
}

func parseTCPAddress(address string) (parsedAddress, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return parsedAddress{}, ErrInvalidAddress
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return parsedAddress{}, ErrInvalidAddress
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	if host == "" {
		return parsedAddress{host: "0.0.0.0", port: port, ip: net.IPv4zero}, nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return parsedAddress{}, ErrInvalidAddress
	}
	if ip4 := ip.To4(); ip4 != nil {
		return parsedAddress{host: ip4.String(), port: port, ip: ip4}, nil
	}
	return parsedAddress{host: ip.String(), port: port, ip: ip.To16(), ipv6: true}, nil
}

func (a parsedAddress) String() string {
	return net.JoinHostPort(a.host, strconv.Itoa(a.port))
}
