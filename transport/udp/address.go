package udp

import (
	"net"
	"strconv"
	"strings"

	"goark.dev/gnalloy/transport"
)

type Address struct {
	IP   net.IP
	Port int
	Zone string
}

func (a Address) Network() string {
	return "udp"
}

func (a Address) String() string {
	host := ""
	if a.IP != nil {
		host = a.IP.String()
	}
	if a.Zone != "" && a.IP != nil && a.IP.To4() == nil {
		host += "%" + a.Zone
	}
	return net.JoinHostPort(host, strconv.Itoa(a.Port))
}

type parsedAddress struct {
	host string
	port int
	ip   net.IP
	zone string
	ipv6 bool
}

func parseAddress(address string) (parsedAddress, error) {
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
	zone := ""
	if i := strings.LastIndexByte(host, '%'); i >= 0 {
		zone = host[i+1:]
		host = host[:i]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return parsedAddress{}, ErrInvalidAddress
	}
	if ip4 := ip.To4(); ip4 != nil {
		return parsedAddress{host: ip4.String(), port: port, ip: ip4}, nil
	}
	return parsedAddress{host: ip.String(), port: port, ip: ip.To16(), zone: zone, ipv6: true}, nil
}

func (a parsedAddress) String() string {
	host := a.host
	if a.zone != "" {
		host += "%" + a.zone
	}
	return net.JoinHostPort(host, strconv.Itoa(a.port))
}

func zoneID(zone string) uint32 {
	if zone == "" {
		return 0
	}
	if n, err := strconv.ParseUint(zone, 10, 32); err == nil {
		return uint32(n)
	}
	if iface, err := net.InterfaceByName(zone); err == nil && iface.Index > 0 {
		return uint32(iface.Index)
	}
	return 0
}

func addressToSocketAddress(addr Address) (transport.SocketAddress, error) {
	if addr.IP == nil || addr.Port < 0 || addr.Port > 65535 {
		return transport.SocketAddress{}, ErrInvalidAddress
	}
	var out transport.SocketAddress
	out.Port = addr.Port
	if ip4 := addr.IP.To4(); ip4 != nil {
		out.Family = transport.SocketFamilyIPv4
		copy(out.IP[:4], ip4)
		return out, nil
	}
	ip16 := addr.IP.To16()
	if ip16 == nil {
		return transport.SocketAddress{}, ErrInvalidAddress
	}
	out.Family = transport.SocketFamilyIPv6
	out.ZoneID = zoneID(addr.Zone)
	copy(out.IP[:], ip16)
	return out, nil
}

func socketAddressToAddress(addr transport.SocketAddress) Address {
	switch addr.Family {
	case transport.SocketFamilyIPv4:
		return Address{IP: net.IPv4(addr.IP[0], addr.IP[1], addr.IP[2], addr.IP[3]), Port: addr.Port}
	case transport.SocketFamilyIPv6:
		zone := ""
		if addr.ZoneID != 0 {
			zone = strconv.Itoa(int(addr.ZoneID))
		}
		ip := make(net.IP, net.IPv6len)
		copy(ip, addr.IP[:])
		return Address{IP: ip, Port: addr.Port, Zone: zone}
	default:
		return Address{}
	}
}
