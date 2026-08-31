//go:build linux

package sctp

import (
	"errors"
	"net"
	"strconv"
	"unsafe"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

const (
	sctpInitMsgOpt = 2
	sctpNoDelayOpt = 3
)

type sctpInitMsg struct {
	numOStreams    uint16
	maxInStreams   uint16
	maxAttempts    uint16
	maxInitTimeout uint16
}

type nativeReadWriter struct{}

func newNativeReadWriter() channel.FDReadWriter {
	return nativeReadWriter{}
}

func (nativeReadWriter) Read(fd transport.FDRef, dst []byte) (int, bool, error) {
	n, err := unix.Read(fd.FD, dst)
	if isAgain(err) {
		return n, true, nil
	}
	return n, false, err
}

func (nativeReadWriter) Write(fd transport.FDRef, src []byte) (int, bool, error) {
	n, err := unix.Write(fd.FD, src)
	if isAgain(err) {
		return n, true, nil
	}
	return n, false, err
}

func (nativeReadWriter) Close(fd transport.FDRef) error {
	return closeFD(fd)
}

func listenSCTP(address string, opts socketOptions) (listenSocket, error) {
	addr, err := parseAddress(address)
	if err != nil {
		return listenSocket{}, err
	}
	family, sa, err := makeSockaddr(addr)
	if err != nil {
		return listenSocket{}, err
	}
	fd, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.IPPROTO_SCTP)
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

func acceptSCTP(fd transport.FDRef) (transport.FDRef, bool, error) {
	nfd, _, err := unix.Accept4(fd.FD, unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC)
	if isAgain(err) {
		return transport.FDRef{}, true, nil
	}
	if err != nil {
		return transport.FDRef{}, false, err
	}
	return transport.FDRef{FD: nfd}, false, nil
}

func dialSCTP(address string, opts socketOptions) (transport.FDRef, error) {
	addr, err := parseAddress(address)
	if err != nil {
		return transport.FDRef{}, err
	}
	family, sa, err := makeSockaddr(addr)
	if err != nil {
		return transport.FDRef{}, err
	}
	fd, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.IPPROTO_SCTP)
	if err != nil {
		return transport.FDRef{}, err
	}
	if err := setPreConnectOptions(fd, opts); err != nil {
		_ = unix.Close(fd)
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

func makeSockaddr(addr parsedAddress) (int, unix.Sockaddr, error) {
	if addr.ipv6 {
		sa := &unix.SockaddrInet6{Port: addr.port}
		copy(sa.Addr[:], addr.ip.To16())
		return unix.AF_INET6, sa, nil
	}
	sa := &unix.SockaddrInet4{Port: addr.port}
	copy(sa.Addr[:], addr.ip.To4())
	return unix.AF_INET, sa, nil
}

func setListenOptions(fd int, family int, opts socketOptions) error {
	if opts.reuseAddr {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			return err
		}
	}
	if family == unix.AF_INET6 {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 1); err != nil {
			return err
		}
	}
	if err := setPreConnectOptions(fd, opts); err != nil {
		return err
	}
	return setConnectedOptions(fd, opts)
}

func setAcceptedOptions(fd transport.FDRef, opts socketOptions) error {
	if opts.keepAlive {
		if err := unix.SetsockoptInt(fd.FD, unix.SOL_SOCKET, unix.SO_KEEPALIVE, 1); err != nil {
			return err
		}
	}
	return setConnectedOptions(fd.FD, opts)
}

func setPreConnectOptions(fd int, opts socketOptions) error {
	msg := sctpInitMsg{
		numOStreams:    opts.outboundStreams,
		maxInStreams:   opts.inboundStreams,
		maxAttempts:    opts.maxInitAttempts,
		maxInitTimeout: opts.maxInitTimeoutMillis,
	}
	_, _, errno := unix.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(fd),
		uintptr(unix.IPPROTO_SCTP),
		uintptr(sctpInitMsgOpt),
		uintptr(unsafe.Pointer(&msg)),
		unsafe.Sizeof(msg),
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func setConnectedOptions(fd int, opts socketOptions) error {
	if opts.noDelay {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_SCTP, sctpNoDelayOpt, 1); err != nil {
			return err
		}
	}
	if opts.sendBufferSize > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, opts.sendBufferSize); err != nil {
			return err
		}
	}
	if opts.receiveBufferSize > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, opts.receiveBufferSize); err != nil {
			return err
		}
	}
	if opts.soLinger >= 0 {
		linger := &unix.Linger{Onoff: 1, Linger: int32(opts.soLinger)}
		if err := unix.SetsockoptLinger(fd, unix.SOL_SOCKET, unix.SO_LINGER, linger); err != nil {
			return err
		}
	}
	return nil
}

func waitConnected(fd int, timeoutMillis int) error {
	timeout := -1
	if timeoutMillis > 0 {
		timeout = timeoutMillis
	}
	for {
		events := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
		n, err := unix.Poll(events, timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrConnectTimeout
		}
		soerr, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
		if err != nil {
			return err
		}
		if soerr != 0 {
			return unix.Errno(soerr)
		}
		return nil
	}
}

func connectInProgress(err error) bool {
	return errors.Is(err, unix.EINPROGRESS) || errors.Is(err, unix.EALREADY)
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
