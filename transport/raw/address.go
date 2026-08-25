package raw

import (
	"net"
	"strconv"
	"strings"

	"goark.dev/gnalloy/transport"
)

// Address 表示 raw packet 的远端或本地 IP 地址。
type Address struct {
	IP   net.IP
	Zone string
}

func (a Address) Network() string {
	if a.IP != nil && a.IP.To4() == nil {
		return "ip6"
	}
	return "ip"
}

func (a Address) String() string {
	if a.IP == nil {
		return ""
	}
	host := a.IP.String()
	if a.Zone != "" && a.IP.To4() == nil {
		host += "%" + a.Zone
	}
	return host
}

type parsedAddress struct {
	host string
	ip   net.IP
	zone string
	ipv6 bool
}

func parseAddress(address string, family Family) (parsedAddress, error) {
	host := strings.TrimSpace(address)
	if host == "" {
		return parsedAddress{}, ErrInvalidAddress
	}
	if host == "localhost" {
		if family == FamilyIPv6 {
			host = "::1"
		} else {
			host = "127.0.0.1"
		}
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
		if family == FamilyIPv6 {
			return parsedAddress{}, ErrInvalidAddress
		}
		return parsedAddress{host: ip4.String(), ip: ip4}, nil
	}
	ip16 := ip.To16()
	if ip16 == nil || family == FamilyIPv4 {
		return parsedAddress{}, ErrInvalidAddress
	}
	return parsedAddress{host: ip.String(), ip: ip16, zone: zone, ipv6: true}, nil
}

func (a parsedAddress) Address() Address {
	return Address{IP: a.ip, Zone: a.zone}
}

func (a parsedAddress) String() string {
	if a.zone == "" {
		return a.host
	}
	return a.host + "%" + a.zone
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
	if addr.IP == nil {
		return transport.SocketAddress{}, ErrInvalidAddress
	}
	var out transport.SocketAddress
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
		return Address{IP: net.IPv4(addr.IP[0], addr.IP[1], addr.IP[2], addr.IP[3])}
	case transport.SocketFamilyIPv6:
		zone := ""
		if addr.ZoneID != 0 {
			zone = strconv.Itoa(int(addr.ZoneID))
		}
		ip := make(net.IP, net.IPv6len)
		copy(ip, addr.IP[:])
		return Address{IP: ip, Zone: zone}
	default:
		return Address{}
	}
}
