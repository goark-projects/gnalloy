//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package tcp

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func listenTCP(string, socketOptions) (listenSocket, error) {
	return listenSocket{}, ErrUnsupportedTCP
}

func acceptTCP(transport.FDRef) (transport.FDRef, bool, error) {
	return transport.FDRef{}, false, ErrUnsupportedTCP
}

func dialTCP(string, socketOptions) (transport.FDRef, error) {
	return transport.FDRef{}, ErrUnsupportedTCP
}

func setAcceptedOptions(transport.FDRef, socketOptions) error {
	return ErrUnsupportedTCP
}

func completeAccepted(transport.FDRef, transport.FDRef) error {
	return ErrUnsupportedTCP
}

func closeFD(transport.FDRef) error {
	return nil
}

func prepareAcceptRequest(req transport.IORequest, _ int) (transport.IORequest, error) {
	return req, ErrUnsupportedCompletionAccept
}

func newNativeReadWriter() channel.FDReadWriter {
	return nil
}
