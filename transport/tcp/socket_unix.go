//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package tcp

import (
	"errors"
	"net"
	"strconv"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/unix"
)

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

func (nativeReadWriter) Writev(fd transport.FDRef, src [][]byte) (int, bool, error) {
	n, err := unix.Writev(fd.FD, src)
	if isAgain(err) {
		return n, true, nil
	}
	return n, false, err
}

func (nativeReadWriter) Close(fd transport.FDRef) error {
	return closeFD(fd)
}

func makeUnixSockaddr(addr parsedAddress) (int, unix.Sockaddr, error) {
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

func setAcceptedOptions(fd transport.FDRef, opts socketOptions) error {
	if opts.noDelay {
		return unix.SetsockoptInt(fd.FD, unix.IPPROTO_TCP, unix.TCP_NODELAY, 1)
	}
	return nil
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

func completeAccepted(transport.FDRef, transport.FDRef) error {
	return nil
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
