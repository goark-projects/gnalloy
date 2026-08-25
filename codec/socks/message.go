package socks

import "net"

const (
	Version4 byte = 0x04
	Version5 byte = 0x05
)

const (
	MethodNoAuth       byte = 0x00
	MethodGSSAPI       byte = 0x01
	MethodUserPassword byte = 0x02
	MethodNoAcceptable byte = 0xff
)

const (
	CommandConnect      byte = 0x01
	CommandBind         byte = 0x02
	CommandUDPAssociate byte = 0x03
)

const (
	AddressIPv4   byte = 0x01
	AddressDomain byte = 0x03
	AddressIPv6   byte = 0x04
)

type Greeting struct {
	Methods []byte
}

type MethodSelection struct {
	Method byte
}

type CommandRequest struct {
	Version byte
	Command byte
	Address string
}

type CommandReply struct {
	Version byte
	Status  byte
	Address string
}

type SOCKS4Request struct {
	Command byte
	Address string
	UserID  string
}

type SOCKS4Reply struct {
	Status  byte
	Address string
}

func splitHostPort(address string) (string, int, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, ErrInvalidAddress
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, ErrInvalidAddress
	}
	return host, port, nil
}
