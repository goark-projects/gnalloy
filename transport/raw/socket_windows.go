//go:build windows

package raw

import (
	"errors"
	"net"
	"strconv"
	"sync"

	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/windows"
)

var wsaStartup struct {
	once sync.Once
	err  error
}

func listenRaw(address string, opts socketOptions) (rawSocket, error) {
	if err := ensureWSAStartup(); err != nil {
		return rawSocket{}, err
	}
	addr, err := parseAddress(address, opts.family)
	if err != nil {
		return rawSocket{}, err
	}
	family, sa := makeWindowsSockaddr(addr)
	fd, err := windows.WSASocket(int32(family), windows.SOCK_RAW, int32(opts.protocol), nil, 0, windows.WSA_FLAG_OVERLAPPED)
	if err != nil {
		return rawSocket{}, err
	}
	if opts.headerIncluded && opts.family == FamilyIPv4 {
		if err := windows.SetsockoptInt(fd, windows.IPPROTO_IP, windows.IP_HDRINCL, 1); err != nil {
			_ = windows.Closesocket(fd)
			return rawSocket{}, err
		}
	}
	if err := windows.Bind(fd, sa); err != nil {
		_ = windows.Closesocket(fd)
		return rawSocket{}, err
	}
	return rawSocket{fd: transport.FDRef{FD: int(fd)}, addr: addr.String(), family: opts.family, protocol: opts.protocol}, nil
}

func recvPacket(fd transport.FDRef, dst []byte) (int, Address, bool, error) {
	n, from, err := windows.Recvfrom(windows.Handle(uintptr(fd.FD)), dst, 0)
	if isAgain(err) {
		return n, Address{}, true, nil
	}
	if err != nil {
		return n, Address{}, false, err
	}
	return n, windowsSockaddrToAddress(from), false, nil
}

func sendPacket(fd transport.FDRef, packet Packet) (bool, error) {
	sa, err := addressToWindowsSockaddr(packet.Addr)
	if err != nil {
		return false, err
	}
	err = windows.Sendto(windows.Handle(uintptr(fd.FD)), packet.Payload.Bytes(), 0, sa)
	if isAgain(err) {
		return true, nil
	}
	return false, err
}

func closeFD(fd transport.FDRef) error {
	if !fd.Valid() {
		return nil
	}
	err := windows.Closesocket(windows.Handle(uintptr(fd.FD)))
	if errors.Is(err, windows.WSAENOTSOCK) {
		return nil
	}
	return err
}

func makeWindowsSockaddr(addr parsedAddress) (int, windows.Sockaddr) {
	if addr.ipv6 {
		sa := &windows.SockaddrInet6{ZoneId: zoneID(addr.zone)}
		copy(sa.Addr[:], addr.ip.To16())
		return windows.AF_INET6, sa
	}
	sa := &windows.SockaddrInet4{}
	copy(sa.Addr[:], addr.ip.To4())
	return windows.AF_INET, sa
}

func addressToWindowsSockaddr(addr Address) (windows.Sockaddr, error) {
	if addr.IP == nil {
		return nil, ErrInvalidAddress
	}
	if ip4 := addr.IP.To4(); ip4 != nil {
		sa := &windows.SockaddrInet4{}
		copy(sa.Addr[:], ip4)
		return sa, nil
	}
	ip16 := addr.IP.To16()
	if ip16 == nil {
		return nil, ErrInvalidAddress
	}
	sa := &windows.SockaddrInet6{ZoneId: zoneID(addr.Zone)}
	copy(sa.Addr[:], ip16)
	return sa, nil
}

func windowsSockaddrToAddress(sa windows.Sockaddr) Address {
	switch v := sa.(type) {
	case *windows.SockaddrInet4:
		return Address{IP: net.IP(v.Addr[:])}
	case *windows.SockaddrInet6:
		zone := ""
		if v.ZoneId != 0 {
			zone = strconv.Itoa(int(v.ZoneId))
		}
		return Address{IP: net.IP(v.Addr[:]), Zone: zone}
	default:
		return Address{}
	}
}

func isAgain(err error) bool {
	return errors.Is(err, windows.WSAEWOULDBLOCK)
}

func ensureWSAStartup() error {
	wsaStartup.once.Do(func() {
		var data windows.WSAData
		wsaStartup.err = windows.WSAStartup(0x202, &data)
	})
	return wsaStartup.err
}
