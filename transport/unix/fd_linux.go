//go:build linux

package unix

import (
	"goark.dev/gnalloy/transport"
	xunix "golang.org/x/sys/unix"
)

// Credentials 是 Unix domain socket 对端进程身份快照。
type Credentials struct {
	PID int32
	UID uint32
	GID uint32
}

// PeerCredentials 读取 Linux SO_PEERCRED 对端进程身份。
func PeerCredentials(fd transport.FDRef) (Credentials, error) {
	if !fd.Valid() {
		return Credentials{}, transport.ErrInvalidFD
	}
	cred, err := xunix.GetsockoptUcred(fd.FD, xunix.SOL_SOCKET, xunix.SO_PEERCRED)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{PID: cred.Pid, UID: cred.Uid, GID: cred.Gid}, nil
}

// SendFD 通过 Unix domain socket 发送一个文件描述符。
func SendFD(socket transport.FDRef, fd int) error {
	if !socket.Valid() || fd < 0 {
		return transport.ErrInvalidFD
	}
	rights := xunix.UnixRights(fd)
	_, err := xunix.SendmsgN(socket.FD, []byte{0}, rights, nil, 0)
	if isAgain(err) {
		return ErrWouldBlock
	}
	return err
}

// ReceiveFD 从 Unix domain socket 接收一个文件描述符。
func ReceiveFD(socket transport.FDRef) (int, error) {
	if !socket.Valid() {
		return -1, transport.ErrInvalidFD
	}
	var data [1]byte
	oob := make([]byte, xunix.CmsgSpace(4))
	_, oobn, _, _, err := xunix.Recvmsg(socket.FD, data[:], oob, 0)
	if isAgain(err) {
		return -1, ErrWouldBlock
	}
	if err != nil {
		return -1, err
	}
	msgs, err := xunix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return -1, err
	}
	for _, msg := range msgs {
		fds, err := xunix.ParseUnixRights(&msg)
		if err != nil {
			return -1, err
		}
		if len(fds) > 0 {
			for _, extra := range fds[1:] {
				_ = xunix.Close(extra)
			}
			return fds[0], nil
		}
	}
	return -1, ErrMissingFileDescriptor
}
