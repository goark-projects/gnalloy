//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package tcp

import (
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

func listenTCP(address string, opts socketOptions) (listenSocket, error) {
	addr, err := parseTCPAddress(address)
	if err != nil {
		return listenSocket{}, err
	}
	family, sa, err := makeUnixSockaddr(addr)
	if err != nil {
		return listenSocket{}, err
	}
	fd, err := unix.Socket(family, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		return listenSocket{}, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return listenSocket{}, err
	}
	unix.CloseOnExec(fd)
	if err := setListenOptions(fd, family, opts); err != nil {
		_ = unix.Close(fd)
		return listenSocket{}, err
	}
	if err := unix.Bind(fd, sa); err != nil {
		_ = unix.Close(fd)
		return listenSocket{}, err
	}
	if err := unix.Listen(fd, opts.backlog); err != nil {
		_ = unix.Close(fd)
		return listenSocket{}, err
	}
	return listenSocket{fd: transport.FDRef{FD: fd}, addr: socketName(fd, addr.String()), family: family}, nil
}

func acceptTCP(fd transport.FDRef) (transport.FDRef, bool, error) {
	nfd, _, err := unix.Accept(fd.FD)
	if isAgain(err) {
		return transport.FDRef{}, true, nil
	}
	if err != nil {
		return transport.FDRef{}, false, err
	}
	if err := unix.SetNonblock(nfd, true); err != nil {
		_ = unix.Close(nfd)
		return transport.FDRef{}, false, err
	}
	unix.CloseOnExec(nfd)
	return transport.FDRef{FD: nfd}, false, nil
}

func prepareAcceptRequest(req transport.IORequest, _ int) (transport.IORequest, error) {
	return req, ErrUnsupportedCompletionAccept
}
