//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package unix

import "goark.dev/gnalloy/transport"

func listenDatagram(string, socketOptions) (datagramSocket, error) {
	return datagramSocket{}, ErrUnsupportedDatagramSocket
}

func sendDatagramTo(transport.FDRef, []byte, Address) (bool, error) {
	return false, ErrUnsupportedDatagramSocket
}

func receiveDatagramFrom(transport.FDRef, []byte) (int, Address, bool, error) {
	return 0, Address{}, false, ErrUnsupportedDatagramSocket
}
