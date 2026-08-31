//go:build !linux

package sctp

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func listenSCTP(string, socketOptions) (listenSocket, error) {
	return listenSocket{}, ErrUnsupportedSCTP
}

func acceptSCTP(transport.FDRef) (transport.FDRef, bool, error) {
	return transport.FDRef{}, false, ErrUnsupportedSCTP
}

func dialSCTP(string, socketOptions) (transport.FDRef, error) {
	return transport.FDRef{}, ErrUnsupportedSCTP
}

func setAcceptedOptions(transport.FDRef, socketOptions) error {
	return ErrUnsupportedSCTP
}

func closeFD(transport.FDRef) error {
	return nil
}

func newNativeReadWriter() channel.FDReadWriter {
	return nil
}
