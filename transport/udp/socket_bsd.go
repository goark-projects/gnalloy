//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package udp

import (
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

func listenUDP(address string, opts socketOptions) (udpSocket, error) {
	addr, err := parseAddress(address)
	if err != nil {
		return udpSocket{}, err
	}
	family, sa, err := makeUnixSockaddr(addr)
	if err != nil {
		return udpSocket{}, err
	}
	fd, err := unix.Socket(family, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return udpSocket{}, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return udpSocket{}, err
	}
	unix.CloseOnExec(fd)
	if err := setSocketOptions(fd, family, opts); err != nil {
		_ = unix.Close(fd)
		return udpSocket{}, err
	}
	if err := unix.Bind(fd, sa); err != nil {
		_ = unix.Close(fd)
		return udpSocket{}, err
	}
	return udpSocket{fd: transport.FDRef{FD: fd}, addr: socketName(fd, addr.String()), family: family}, nil
}
