//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package udp

import (
	"errors"
	"net"
	"strconv"

	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

func makeUnixSockaddr(addr parsedAddress) (int, unix.Sockaddr, error) {
	if addr.ipv6 {
		sa := &unix.SockaddrInet6{Port: addr.port, ZoneId: zoneID(addr.zone)}
		copy(sa.Addr[:], addr.ip.To16())
		return unix.AF_INET6, sa, nil
	}
	sa := &unix.SockaddrInet4{Port: addr.port}
	copy(sa.Addr[:], addr.ip.To4())
	return unix.AF_INET, sa, nil
}

func addressToUnixSockaddr(addr Address) (unix.Sockaddr, error) {
	if addr.IP == nil || addr.Port < 0 || addr.Port > 65535 {
		return nil, ErrInvalidAddress
	}
	if ip4 := addr.IP.To4(); ip4 != nil {
		sa := &unix.SockaddrInet4{Port: addr.Port}
		copy(sa.Addr[:], ip4)
		return sa, nil
	}
	ip16 := addr.IP.To16()
	if ip16 == nil {
		return nil, ErrInvalidAddress
	}
	sa := &unix.SockaddrInet6{Port: addr.Port, ZoneId: zoneID(addr.Zone)}
	copy(sa.Addr[:], ip16)
	return sa, nil
}

func unixSockaddrToAddress(sa unix.Sockaddr) Address {
	switch v := sa.(type) {
	case *unix.SockaddrInet4:
		return Address{IP: net.IP(v.Addr[:]), Port: v.Port}
	case *unix.SockaddrInet6:
		zone := ""
		if v.ZoneId != 0 {
			zone = strconv.Itoa(int(v.ZoneId))
		}
		return Address{IP: net.IP(v.Addr[:]), Port: v.Port, Zone: zone}
	default:
		return Address{}
	}
}

func setSocketOptions(fd int, family int, opts socketOptions) error {
	if opts.reuseAddr {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			return err
		}
	}
	if opts.reusePort {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
			return err
		}
	}
	if family == unix.AF_INET6 {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 1); err != nil {
			return err
		}
	}
	return nil
}

func recvDatagram(fd transport.FDRef, dst []byte) (int, Address, bool, error) {
	n, from, err := unix.Recvfrom(fd.FD, dst, 0)
	if isAgain(err) {
		return n, Address{}, true, nil
	}
	if err != nil {
		return n, Address{}, false, err
	}
	return n, unixSockaddrToAddress(from), false, nil
}

func sendDatagram(fd transport.FDRef, datagram Datagram) (bool, error) {
	sa, err := addressToUnixSockaddr(datagram.Addr)
	if err != nil {
		return false, err
	}
	err = unix.Sendto(fd.FD, datagram.Payload.Bytes(), 0, sa)
	if isAgain(err) {
		return true, nil
	}
	return false, err
}

func closeFD(fd transport.FDRef) error {
	if !fd.Valid() {
		return nil
	}
	err := unix.Close(fd.FD)
	if errors.Is(err, unix.EBADF) {
		return nil
	}
	return err
}

func socketName(fd int, fallback string) string {
	sa, err := unix.Getsockname(fd)
	if err != nil {
		return fallback
	}
	switch v := sa.(type) {
	case *unix.SockaddrInet4:
		return net.JoinHostPort(net.IP(v.Addr[:]).String(), strconv.Itoa(v.Port))
	case *unix.SockaddrInet6:
		return net.JoinHostPort(net.IP(v.Addr[:]).String(), strconv.Itoa(v.Port))
	default:
		return fallback
	}
}

func isAgain(err error) bool {
	return errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK)
}
