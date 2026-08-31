//go:build !linux

package unix

import "goark.dev/gnalloy/transport"

// Credentials 是 Unix domain socket 对端进程身份快照。
type Credentials struct {
	PID int32
	UID uint32
	GID uint32
}

func PeerCredentials(transport.FDRef) (Credentials, error) {
	return Credentials{}, ErrUnsupportedPeerCredentials
}

func SendFD(transport.FDRef, int) error {
	return ErrUnsupportedFDPassing
}

func ReceiveFD(transport.FDRef) (int, error) {
	return -1, ErrUnsupportedFDPassing
}
