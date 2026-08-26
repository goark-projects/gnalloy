//go:build windows

package tcp

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func connectWindows(fd windows.Handle, family int, remote windows.Sockaddr, timeoutMillis int) error {
	if err := windows.Bind(fd, localWindowsSockaddr(family)); err != nil {
		return err
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)

	var overlapped windows.Overlapped
	overlapped.HEvent = event
	err = windows.ConnectEx(fd, remote, nil, 0, nil, &overlapped)
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		return err
	}
	if err == nil {
		return updateConnectContext(fd)
	}

	wait, err := windows.WaitForSingleObject(event, windowsConnectTimeout(timeoutMillis))
	if err != nil {
		return err
	}
	if wait == uint32(windows.WAIT_TIMEOUT) {
		_ = windows.CancelIoEx(fd, &overlapped)
		return ErrConnectTimeout
	}
	if wait != uint32(windows.WAIT_OBJECT_0) {
		return syscall.Errno(wait)
	}
	var transferred uint32
	if err := windows.GetOverlappedResult(fd, &overlapped, &transferred, false); err != nil {
		return err
	}
	return updateConnectContext(fd)
}

func localWindowsSockaddr(family int) windows.Sockaddr {
	if family == windows.AF_INET6 {
		return &windows.SockaddrInet6{}
	}
	return &windows.SockaddrInet4{}
}

func windowsConnectTimeout(timeoutMillis int) uint32 {
	if timeoutMillis <= 0 {
		return windows.INFINITE
	}
	return uint32(timeoutMillis)
}

func updateConnectContext(fd windows.Handle) error {
	return windows.Setsockopt(fd, windows.SOL_SOCKET, windows.SO_UPDATE_CONNECT_CONTEXT, nil, 0)
}
