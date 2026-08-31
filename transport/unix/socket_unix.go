//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package unix

import (
	"errors"
	"os"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	xunix "golang.org/x/sys/unix"
)

type nativeReadWriter struct{}

func newNativeReadWriter() channel.FDReadWriter {
	return nativeReadWriter{}
}

func (nativeReadWriter) Read(fd transport.FDRef, dst []byte) (int, bool, error) {
	n, err := xunix.Read(fd.FD, dst)
	if isAgain(err) {
		return n, true, nil
	}
	return n, false, err
}

func (nativeReadWriter) Write(fd transport.FDRef, src []byte) (int, bool, error) {
	n, err := xunix.Write(fd.FD, src)
	if isAgain(err) {
		return n, true, nil
	}
	return n, false, err
}

func (nativeReadWriter) Writev(fd transport.FDRef, src [][]byte) (int, bool, error) {
	n, err := xunix.Writev(fd.FD, src)
	if isAgain(err) {
		return n, true, nil
	}
	return n, false, err
}

func (nativeReadWriter) Close(fd transport.FDRef) error {
	return closeFD(fd)
}

func listenUnix(address string, opts socketOptions) (listenSocket, error) {
	addr, err := ParseAddress(address)
	if err != nil {
		return listenSocket{}, err
	}
	if addr.Abstract && !abstractSocketSupported() {
		return listenSocket{}, ErrUnsupportedAbstractSocket
	}
	if opts.removeStaleSocket && !addr.Abstract {
		if err := os.Remove(addr.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return listenSocket{}, err
		}
	}
	fd, err := xunix.Socket(xunix.AF_UNIX, xunix.SOCK_STREAM, 0)
	if err != nil {
		return listenSocket{}, err
	}
	xunix.CloseOnExec(fd)
	if err := xunix.SetNonblock(fd, true); err != nil {
		_ = xunix.Close(fd)
		return listenSocket{}, err
	}
	if err := xunix.Bind(fd, &xunix.SockaddrUnix{Name: addr.sockaddrName()}); err != nil {
		_ = xunix.Close(fd)
		return listenSocket{}, err
	}
	if opts.fileMode != 0 && !addr.Abstract {
		if err := os.Chmod(addr.Path, opts.fileMode); err != nil {
			_ = xunix.Close(fd)
			_ = cleanupSocket(addr)
			return listenSocket{}, err
		}
	}
	if err := xunix.Listen(fd, opts.backlog); err != nil {
		_ = xunix.Close(fd)
		_ = cleanupSocket(addr)
		return listenSocket{}, err
	}
	return listenSocket{fd: transport.FDRef{FD: fd}, addr: addr}, nil
}

func acceptUnix(fd transport.FDRef) (transport.FDRef, bool, error) {
	nfd, _, err := xunix.Accept(fd.FD)
	if isAgain(err) {
		return transport.FDRef{FD: -1}, true, nil
	}
	if err != nil {
		return transport.FDRef{FD: -1}, false, err
	}
	if err := xunix.SetNonblock(nfd, true); err != nil {
		_ = xunix.Close(nfd)
		return transport.FDRef{FD: -1}, false, err
	}
	xunix.CloseOnExec(nfd)
	return transport.FDRef{FD: nfd}, false, nil
}

func dialUnix(address string, _ socketOptions) (transport.FDRef, error) {
	addr, err := ParseAddress(address)
	if err != nil {
		return transport.FDRef{FD: -1}, err
	}
	if addr.Abstract && !abstractSocketSupported() {
		return transport.FDRef{FD: -1}, ErrUnsupportedAbstractSocket
	}
	fd, err := xunix.Socket(xunix.AF_UNIX, xunix.SOCK_STREAM, 0)
	if err != nil {
		return transport.FDRef{FD: -1}, err
	}
	xunix.CloseOnExec(fd)
	if err := xunix.SetNonblock(fd, true); err != nil {
		_ = xunix.Close(fd)
		return transport.FDRef{FD: -1}, err
	}
	err = xunix.Connect(fd, &xunix.SockaddrUnix{Name: addr.sockaddrName()})
	if err != nil && !isConnectInProgress(err) {
		_ = xunix.Close(fd)
		return transport.FDRef{FD: -1}, err
	}
	if err := waitConnected(fd); err != nil {
		_ = xunix.Close(fd)
		return transport.FDRef{FD: -1}, err
	}
	return transport.FDRef{FD: fd}, nil
}

func cleanupSocket(addr Address) error {
	if addr.Path == "" || addr.Abstract {
		return nil
	}
	err := os.Remove(addr.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func chmodSocket(addr Address, mode os.FileMode) error {
	return os.Chmod(addr.Path, mode)
}

func closeFD(fd transport.FDRef) error {
	if !fd.Valid() {
		return nil
	}
	err := xunix.Close(fd.FD)
	if errors.Is(err, xunix.EBADF) {
		return nil
	}
	return err
}

func waitConnected(fd int) error {
	for {
		events := []xunix.PollFd{{Fd: int32(fd), Events: xunix.POLLOUT}}
		n, err := xunix.Poll(events, -1)
		if errors.Is(err, xunix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		soerr, err := xunix.GetsockoptInt(fd, xunix.SOL_SOCKET, xunix.SO_ERROR)
		if err != nil {
			return err
		}
		if soerr != 0 {
			return xunix.Errno(soerr)
		}
		return nil
	}
}

func isConnectInProgress(err error) bool {
	return errors.Is(err, xunix.EINPROGRESS) || errors.Is(err, xunix.EALREADY)
}

func isAgain(err error) bool {
	return errors.Is(err, xunix.EAGAIN) || errors.Is(err, xunix.EWOULDBLOCK)
}
