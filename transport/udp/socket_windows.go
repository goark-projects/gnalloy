//go:build windows

package udp

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

func listenUDP(address string, opts socketOptions) (udpSocket, error) {
	if err := ensureWSAStartup(); err != nil {
		return udpSocket{}, err
	}
	addr, err := parseAddress(address)
	if err != nil {
		return udpSocket{}, err
	}
	family, sa := makeWindowsSockaddr(addr)
	fd, err := windows.WSASocket(int32(family), windows.SOCK_DGRAM, windows.IPPROTO_UDP, nil, 0, windows.WSA_FLAG_OVERLAPPED)
	if err != nil {
		return udpSocket{}, err
	}
	if opts.reuseAddr {
		if err := windows.SetsockoptInt(fd, windows.SOL_SOCKET, windows.SO_REUSEADDR, 1); err != nil {
			_ = windows.Closesocket(fd)
			return udpSocket{}, err
		}
	}
	if err := windows.Bind(fd, sa); err != nil {
		_ = windows.Closesocket(fd)
		return udpSocket{}, err
	}
	return udpSocket{fd: transport.FDRef{FD: int(fd)}, addr: socketName(fd, addr.String()), family: family}, nil
}

func recvDatagram(fd transport.FDRef, dst []byte) (int, Address, bool, error) {
	n, from, err := windows.Recvfrom(windows.Handle(uintptr(fd.FD)), dst, 0)
	if isAgain(err) {
		return n, Address{}, true, nil
	}
	if err != nil {
		return n, Address{}, false, err
	}
	return n, windowsSockaddrToAddress(from), false, nil
}

func sendDatagram(fd transport.FDRef, datagram Datagram) (bool, error) {
	sa, err := addressToWindowsSockaddr(datagram.Addr)
	if err != nil {
		return false, err
	}
	err = windows.Sendto(windows.Handle(uintptr(fd.FD)), datagram.Payload.Bytes(), 0, sa)
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
		sa := &windows.SockaddrInet6{Port: addr.port, ZoneId: zoneID(addr.zone)}
		copy(sa.Addr[:], addr.ip.To16())
		return windows.AF_INET6, sa
	}
	sa := &windows.SockaddrInet4{Port: addr.port}
	copy(sa.Addr[:], addr.ip.To4())
	return windows.AF_INET, sa
}

func addressToWindowsSockaddr(addr Address) (windows.Sockaddr, error) {
	if addr.IP == nil || addr.Port < 0 || addr.Port > 65535 {
		return nil, ErrInvalidAddress
	}
	if ip4 := addr.IP.To4(); ip4 != nil {
		sa := &windows.SockaddrInet4{Port: addr.Port}
		copy(sa.Addr[:], ip4)
		return sa, nil
	}
	ip16 := addr.IP.To16()
	if ip16 == nil {
		return nil, ErrInvalidAddress
	}
	sa := &windows.SockaddrInet6{Port: addr.Port, ZoneId: zoneID(addr.Zone)}
	copy(sa.Addr[:], ip16)
	return sa, nil
}

func windowsSockaddrToAddress(sa windows.Sockaddr) Address {
	switch v := sa.(type) {
	case *windows.SockaddrInet4:
		return Address{IP: net.IP(v.Addr[:]), Port: v.Port}
	case *windows.SockaddrInet6:
		zone := ""
		if v.ZoneId != 0 {
			zone = strconv.Itoa(int(v.ZoneId))
		}
		return Address{IP: net.IP(v.Addr[:]), Port: v.Port, Zone: zone}
	default:
		return Address{}
	}
}

func socketName(fd windows.Handle, fallback string) string {
	sa, err := windows.Getsockname(fd)
	if err != nil {
		return fallback
	}
	switch v := sa.(type) {
	case *windows.SockaddrInet4:
		return net.JoinHostPort(net.IP(v.Addr[:]).String(), strconv.Itoa(v.Port))
	case *windows.SockaddrInet6:
		return net.JoinHostPort(net.IP(v.Addr[:]).String(), strconv.Itoa(v.Port))
	default:
		return fallback
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
