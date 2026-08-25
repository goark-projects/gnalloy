//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package raw

import "goark.dev/gnalloy/transport"

func listenRaw(string, socketOptions) (rawSocket, error) {
	return rawSocket{}, transport.ErrUnsupportedPoller
}

func recvPacket(transport.FDRef, []byte) (int, Address, bool, error) {
	return 0, Address{}, false, transport.ErrUnsupportedPoller
}

func sendPacket(transport.FDRef, Packet) (bool, error) {
	return false, transport.ErrUnsupportedPoller
}

func closeFD(transport.FDRef) error {
	return nil
}
