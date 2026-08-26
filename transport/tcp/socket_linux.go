//go:build linux

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
	fd, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.IPPROTO_TCP)
	if err != nil {
		return listenSocket{}, err
	}
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
	nfd, _, err := unix.Accept4(fd.FD, unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC)
	if isAgain(err) {
		return transport.FDRef{}, true, nil
	}
	if err != nil {
		return transport.FDRef{}, false, err
	}
	return transport.FDRef{FD: nfd}, false, nil
}

func dialTCP(address string, opts socketOptions) (transport.FDRef, error) {
	addr, err := parseTCPAddress(address)
	if err != nil {
		return transport.FDRef{}, err
	}
	family, sa, err := makeUnixSockaddr(addr)
	if err != nil {
		return transport.FDRef{}, err
	}
	fd, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.IPPROTO_TCP)
	if err != nil {
		return transport.FDRef{}, err
	}
	if err := unix.Connect(fd, sa); err != nil && !connectInProgress(err) {
		_ = unix.Close(fd)
		return transport.FDRef{}, err
	}
	if err := waitConnected(fd, opts.connectTimeoutMillis); err != nil {
		_ = unix.Close(fd)
		return transport.FDRef{}, err
	}
	ref := transport.FDRef{FD: fd}
	if err := setAcceptedOptions(ref, opts); err != nil {
		_ = closeFD(ref)
		return transport.FDRef{}, err
	}
	return ref, nil
}

func prepareAcceptRequest(req transport.IORequest, _ int) (transport.IORequest, error) {
	return req, nil
}
