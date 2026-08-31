package unix

import (
	"sync"

	"goark.dev/gnalloy/transport"
)

type datagramSocket struct {
	fd   transport.FDRef
	addr Address
}

// DatagramEndpoint 表示绑定到 Unix domain datagram socket 的轻量端点。
type DatagramEndpoint struct {
	fd   transport.FDRef
	addr Address

	once sync.Once
	err  error
}

// ListenDatagram 绑定 Unix domain datagram socket。
func ListenDatagram(address string, cfg Config) (*DatagramEndpoint, error) {
	opts := normalizeConfig(cfg).socketOptions()
	socket, err := listenDatagram(address, opts)
	if err != nil {
		return nil, err
	}
	return &DatagramEndpoint{fd: socket.fd, addr: socket.addr}, nil
}

func (e *DatagramEndpoint) Addr() Address {
	if e == nil {
		return Address{}
	}
	return e.addr
}

func (e *DatagramEndpoint) FD() transport.FDRef {
	if e == nil {
		return transport.FDRef{FD: -1}
	}
	return e.fd
}

// SendTo 向目标 Unix domain datagram 地址发送一条完整 datagram。
func (e *DatagramEndpoint) SendTo(payload []byte, address string) error {
	if e == nil || !e.fd.Valid() {
		return transport.ErrInvalidFD
	}
	addr, err := ParseAddress(address)
	if err != nil {
		return err
	}
	again, err := sendDatagramTo(e.fd, payload, addr)
	if again {
		return ErrWouldBlock
	}
	return err
}

// ReceiveFrom 从端点读取一条 datagram，并返回发送端地址。
func (e *DatagramEndpoint) ReceiveFrom(dst []byte) (int, Address, error) {
	if e == nil || !e.fd.Valid() {
		return 0, Address{}, transport.ErrInvalidFD
	}
	n, from, again, err := receiveDatagramFrom(e.fd, dst)
	if again {
		return n, Address{}, ErrWouldBlock
	}
	return n, from, err
}

func (e *DatagramEndpoint) Close() error {
	if e == nil {
		return nil
	}
	e.once.Do(func() {
		e.err = closeFD(e.fd)
		if err := cleanupSocket(e.addr); err != nil && e.err == nil {
			e.err = err
		}
		e.fd = transport.FDRef{FD: -1}
	})
	return e.err
}
