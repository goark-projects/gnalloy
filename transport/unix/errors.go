package unix

import "errors"

var (
	ErrUnsupportedUnixSocket      = errors.New("gnalloy/transport/unix: unsupported unix domain socket")
	ErrUnsupportedAbstractSocket  = errors.New("gnalloy/transport/unix: unsupported abstract socket")
	ErrInvalidAddress             = errors.New("gnalloy/transport/unix: invalid address")
	ErrPathTooLong                = errors.New("gnalloy/transport/unix: socket path too long")
	ErrServerClosed               = errors.New("gnalloy/transport/unix: server closed")
	ErrCloseActiveTimeout         = errors.New("gnalloy/transport/unix: close active timeout")
	ErrUnsupportedFixedBuffers    = errors.New("gnalloy/transport/unix: unsupported fixed buffers")
	ErrUnsupportedDatagramSocket  = errors.New("gnalloy/transport/unix: unsupported datagram socket")
	ErrUnsupportedPeerCredentials = errors.New("gnalloy/transport/unix: unsupported peer credentials")
	ErrUnsupportedFDPassing       = errors.New("gnalloy/transport/unix: unsupported fd passing")
	ErrMissingFileDescriptor      = errors.New("gnalloy/transport/unix: missing file descriptor")
	ErrWouldBlock                 = errors.New("gnalloy/transport/unix: operation would block")
)
