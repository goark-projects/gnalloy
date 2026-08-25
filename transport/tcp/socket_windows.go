//go:build windows

package tcp

import (
	"errors"
	"net"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"golang.org/x/sys/windows"
)

var wsaStartup struct {
	once sync.Once
	err  error
}

type nativeReadWriter struct{}

func newNativeReadWriter() channel.FDReadWriter {
	return nativeReadWriter{}
}

func (nativeReadWriter) Read(fd transport.FDRef, dst []byte) (int, bool, error) {
	if len(dst) == 0 {
		return 0, false, nil
	}
	buf := windows.WSABuf{Len: uint32(len(dst)), Buf: &dst[0]}
	var flags uint32
	var recvd uint32
	err := windows.WSARecv(windows.Handle(uintptr(fd.FD)), &buf, 1, &recvd, &flags, nil, nil)
	if isAgain(err) {
		return int(recvd), true, nil
	}
	return int(recvd), false, err
}

func (nativeReadWriter) Write(fd transport.FDRef, src []byte) (int, bool, error) {
	if len(src) == 0 {
		return 0, false, nil
	}
	buf := windows.WSABuf{Len: uint32(len(src)), Buf: &src[0]}
	var sent uint32
	err := windows.WSASend(windows.Handle(uintptr(fd.FD)), &buf, 1, &sent, 0, nil, nil)
	if isAgain(err) {
		return int(sent), true, nil
	}
	return int(sent), false, err
}

func (nativeReadWriter) Writev(fd transport.FDRef, src [][]byte) (int, bool, error) {
	wsabufs := make([]windows.WSABuf, 0, len(src))
	for _, part := range src {
		if len(part) == 0 {
			continue
		}
		wsabufs = append(wsabufs, windows.WSABuf{Len: uint32(len(part)), Buf: &part[0]})
	}
	if len(wsabufs) == 0 {
		return 0, false, nil
	}
	var sent uint32
	err := windows.WSASend(windows.Handle(uintptr(fd.FD)), &wsabufs[0], uint32(len(wsabufs)), &sent, 0, nil, nil)
	if isAgain(err) {
		return int(sent), true, nil
	}
	return int(sent), false, err
}

func (nativeReadWriter) Close(fd transport.FDRef) error {
	return closeFD(fd)
}

func listenTCP(address string, opts socketOptions) (listenSocket, error) {
	if err := ensureWSAStartup(); err != nil {
		return listenSocket{}, err
	}
	addr, err := parseTCPAddress(address)
	if err != nil {
		return listenSocket{}, err
	}
	family, sa := makeWindowsSockaddr(addr)
	fd, err := windows.WSASocket(int32(family), windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_OVERLAPPED)
	if err != nil {
		return listenSocket{}, err
	}
	if opts.reuseAddr {
		if err := windows.SetsockoptInt(fd, windows.SOL_SOCKET, windows.SO_REUSEADDR, 1); err != nil {
			_ = windows.Closesocket(fd)
			return listenSocket{}, err
		}
	}
	if err := windows.Bind(fd, sa); err != nil {
		_ = windows.Closesocket(fd)
		return listenSocket{}, err
	}
	if err := windows.Listen(fd, opts.backlog); err != nil {
		_ = windows.Closesocket(fd)
		return listenSocket{}, err
	}
	return listenSocket{fd: transport.FDRef{FD: int(fd)}, addr: socketName(fd, addr.String()), family: family}, nil
}

func acceptTCP(transport.FDRef) (transport.FDRef, bool, error) {
	return transport.FDRef{}, false, ErrUnsupportedCompletionAccept
}

func dialTCP(address string, opts socketOptions) (transport.FDRef, error) {
	if err := ensureWSAStartup(); err != nil {
		return transport.FDRef{}, err
	}
	addr, err := parseTCPAddress(address)
	if err != nil {
		return transport.FDRef{}, err
	}
	family, sa := makeWindowsSockaddr(addr)
	fd, err := windows.WSASocket(int32(family), windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_OVERLAPPED)
	if err != nil {
		return transport.FDRef{}, err
	}
	if err := windows.Connect(fd, sa); err != nil {
		_ = windows.Closesocket(fd)
		return transport.FDRef{}, err
	}
	if err := syscall.SetNonblock(syscall.Handle(fd), true); err != nil {
		_ = windows.Closesocket(fd)
		return transport.FDRef{}, err
	}
	ref := transport.FDRef{FD: int(fd)}
	if err := setAcceptedOptions(ref, opts); err != nil {
		_ = closeFD(ref)
		return transport.FDRef{}, err
	}
	return ref, nil
}

func setAcceptedOptions(fd transport.FDRef, opts socketOptions) error {
	if opts.noDelay {
		return windows.SetsockoptInt(windows.Handle(uintptr(fd.FD)), windows.IPPROTO_TCP, windows.TCP_NODELAY, 1)
	}
	return nil
}

func completeAccepted(listenFD transport.FDRef, acceptedFD transport.FDRef) error {
	listen := windows.Handle(uintptr(listenFD.FD))
	return windows.Setsockopt(
		windows.Handle(uintptr(acceptedFD.FD)),
		windows.SOL_SOCKET,
		windows.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&listen)),
		int32(unsafe.Sizeof(listen)),
	)
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

func prepareAcceptRequest(req transport.IORequest, family int) (transport.IORequest, error) {
	if err := ensureWSAStartup(); err != nil {
		return req, err
	}
	fd, err := windows.WSASocket(int32(family), windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_OVERLAPPED)
	if err != nil {
		return req, err
	}
	req.AcceptedFD = transport.FDRef{FD: int(fd)}
	return req, nil
}

func makeWindowsSockaddr(addr parsedAddress) (int, windows.Sockaddr) {
	if addr.ipv6 {
		sa := &windows.SockaddrInet6{Port: addr.port}
		copy(sa.Addr[:], addr.ip.To16())
		return windows.AF_INET6, sa
	}
	sa := &windows.SockaddrInet4{Port: addr.port}
	copy(sa.Addr[:], addr.ip.To4())
	return windows.AF_INET, sa
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
