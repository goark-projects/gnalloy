//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package unix

import (
	"strings"

	"goark.dev/gnalloy/transport"
	xunix "golang.org/x/sys/unix"
)

func listenDatagram(address string, opts socketOptions) (datagramSocket, error) {
	addr, err := ParseAddress(address)
	if err != nil {
		return datagramSocket{}, err
	}
	if addr.Abstract && !abstractSocketSupported() {
		return datagramSocket{}, ErrUnsupportedAbstractSocket
	}
	if opts.removeStaleSocket && !addr.Abstract {
		if err := cleanupSocket(addr); err != nil {
			return datagramSocket{}, err
		}
	}
	fd, err := xunix.Socket(xunix.AF_UNIX, xunix.SOCK_DGRAM, 0)
	if err != nil {
		return datagramSocket{}, err
	}
	xunix.CloseOnExec(fd)
	if err := xunix.SetNonblock(fd, true); err != nil {
		_ = xunix.Close(fd)
		return datagramSocket{}, err
	}
	if err := xunix.Bind(fd, &xunix.SockaddrUnix{Name: addr.sockaddrName()}); err != nil {
		_ = xunix.Close(fd)
		return datagramSocket{}, err
	}
	if opts.fileMode != 0 && !addr.Abstract {
		if err := chmodSocket(addr, opts.fileMode); err != nil {
			_ = xunix.Close(fd)
			_ = cleanupSocket(addr)
			return datagramSocket{}, err
		}
	}
	return datagramSocket{fd: transport.FDRef{FD: fd}, addr: addr}, nil
}

func sendDatagramTo(fd transport.FDRef, payload []byte, addr Address) (bool, error) {
	if addr.Abstract && !abstractSocketSupported() {
		return false, ErrUnsupportedAbstractSocket
	}
	err := xunix.Sendto(fd.FD, payload, 0, &xunix.SockaddrUnix{Name: addr.sockaddrName()})
	if isAgain(err) {
		return true, nil
	}
	return false, err
}

func receiveDatagramFrom(fd transport.FDRef, dst []byte) (int, Address, bool, error) {
	n, from, err := xunix.Recvfrom(fd.FD, dst, 0)
	if isAgain(err) {
		return n, Address{}, true, nil
	}
	if err != nil {
		return n, Address{}, false, err
	}
	return n, sockaddrUnixToAddress(from), false, nil
}

func sockaddrUnixToAddress(sa xunix.Sockaddr) Address {
	addr, ok := sa.(*xunix.SockaddrUnix)
	if !ok || addr.Name == "" {
		return Address{}
	}
	if strings.HasPrefix(addr.Name, "\x00") {
		return Address{Path: strings.TrimPrefix(addr.Name, "\x00"), Abstract: true}
	}
	return Address{Path: addr.Name}
}
