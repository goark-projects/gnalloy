//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package udp

import "goark.dev/gnalloy/transport"

func listenUDP(string, socketOptions) (udpSocket, error) {
	return udpSocket{}, transport.ErrUnsupportedPoller
}

func recvDatagram(transport.FDRef, []byte) (int, Address, bool, error) {
	return 0, Address{}, false, transport.ErrUnsupportedPoller
}

func sendDatagram(transport.FDRef, Datagram) (bool, error) {
	return false, transport.ErrUnsupportedPoller
}

func closeFD(transport.FDRef) error {
	return nil
}
