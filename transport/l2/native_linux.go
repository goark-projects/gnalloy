//go:build linux

package l2

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"goark.dev/gnalloy/buffer"
	"golang.org/x/sys/unix"
)

const etherTypeAll = 0x0003

type nativeDriver struct{}

// DefaultDriverKind 返回当前平台默认 L2 driver。
func DefaultDriverKind() DriverKind {
	return DriverKindAFPacket
}

func (nativeDriver) Open(_ context.Context, cfg Config) (Endpoint, error) {
	iface, err := resolveInterface(cfg)
	if err != nil {
		return nil, err
	}
	protocol := cfg.EtherType
	if protocol == 0 {
		protocol = etherTypeAll
	}
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, int(htons(protocol)))
	if err != nil {
		return nil, err
	}
	addr := &unix.SockaddrLinklayer{Protocol: htons(protocol), Ifindex: iface.Index}
	if err := unix.Bind(fd, addr); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if cfg.Promiscuous {
		if err := enablePromiscuous(fd, iface.Index); err != nil {
			_ = unix.Close(fd)
			return nil, err
		}
	}
	return &afPacketEndpoint{fd: fd, iface: iface, protocol: protocol}, nil
}

type afPacketEndpoint struct {
	fd       int
	iface    *net.Interface
	protocol uint16
}

func (e *afPacketEndpoint) Addr() string {
	if e == nil || e.iface == nil {
		return ""
	}
	return e.iface.Name
}

func (e *afPacketEndpoint) ReadFrame(_ context.Context, alloc buffer.Allocator, readBufferSize int) (Frame, error) {
	if alloc == nil {
		return Frame{}, ErrInvalidConfig
	}
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	buf, err := alloc.Acquire(readBufferSize)
	if err != nil {
		return Frame{}, err
	}
	n, _, err := unix.Recvfrom(e.fd, buf.WritableBytesView(), 0)
	if err != nil {
		buf.Release()
		return Frame{}, err
	}
	if err := buf.AdvanceWriter(n); err != nil {
		buf.Release()
		return Frame{}, err
	}
	return Frame{Meta: parseEthernetMeta(e.iface, buf.Bytes()), Payload: buf}, nil
}

func (e *afPacketEndpoint) WriteFrame(_ context.Context, frame Frame) error {
	if !frame.Valid() {
		return ErrInvalidFrame
	}
	data := frame.Payload.Bytes()
	for len(data) > 0 {
		n, err := unix.Write(e.fd, data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return ErrInvalidFrame
		}
		data = data[n:]
	}
	return nil
}

func (e *afPacketEndpoint) Close() error {
	if e == nil || e.fd < 0 {
		return nil
	}
	err := unix.Close(e.fd)
	e.fd = -1
	return err
}

func resolveInterface(cfg Config) (*net.Interface, error) {
	if cfg.InterfaceName != "" {
		return net.InterfaceByName(cfg.InterfaceName)
	}
	if cfg.InterfaceIndex > 0 {
		return net.InterfaceByIndex(cfg.InterfaceIndex)
	}
	return nil, fmt.Errorf("%w: missing interface", ErrInvalidConfig)
}

func htons(v uint16) uint16 {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return binary.NativeEndian.Uint16(b[:])
}

func enablePromiscuous(fd int, ifindex int) error {
	mreq := &unix.PacketMreq{
		Ifindex: int32(ifindex),
		Type:    unix.PACKET_MR_PROMISC,
	}
	return unix.SetsockoptPacketMreq(fd, unix.SOL_PACKET, unix.PACKET_ADD_MEMBERSHIP, mreq)
}

func parseEthernetMeta(iface *net.Interface, data []byte) FrameMeta {
	meta := FrameMeta{}
	if iface != nil {
		meta.InterfaceName = iface.Name
		meta.InterfaceIndex = iface.Index
	}
	if len(data) >= 14 {
		meta.Destination = append(net.HardwareAddr(nil), data[0:6]...)
		meta.Source = append(net.HardwareAddr(nil), data[6:12]...)
		meta.EtherType = binary.BigEndian.Uint16(data[12:14])
	}
	return meta
}
