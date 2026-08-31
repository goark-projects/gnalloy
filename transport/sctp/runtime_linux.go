//go:build linux

package sctp

import "golang.org/x/sys/unix"

// detectRuntimeSupport 返回 Linux SCTP socket 的构建期能力边界。
func detectRuntimeSupport() RuntimeSupport {
	support := RuntimeSupport{
		Platform:         runtimePlatform(),
		NativeSocket:     true,
		ReadinessPoller:  true,
		CompletionPoller: false,
		OneToOneStream:   true,
		InitMessage:      true,
		NoDelay:          true,
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_SCTP)
	if err != nil {
		return support
	}
	_ = unix.Close(fd)
	support.KernelAvailable = true
	return support
}
