//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package raw

import (
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

func listenRaw(address string, opts socketOptions) (rawSocket, error) {
	addr, err := parseAddress(address, opts.family)
	if err != nil {
		return rawSocket{}, err
	}
	family, sa, err := makeUnixSockaddr(addr)
	if err != nil {
		return rawSocket{}, err
	}
	fd, err := unix.Socket(family, unix.SOCK_RAW, opts.protocol)
	if err != nil {
		return rawSocket{}, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return rawSocket{}, err
	}
	unix.CloseOnExec(fd)
	if err := setSocketOptions(fd, opts.family, opts); err != nil {
		_ = unix.Close(fd)
		return rawSocket{}, err
	}
	if err := unix.Bind(fd, sa); err != nil {
		_ = unix.Close(fd)
		return rawSocket{}, err
	}
	return rawSocket{fd: transport.FDRef{FD: fd}, addr: addr.String(), family: opts.family, protocol: opts.protocol}, nil
}
