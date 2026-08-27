package nativeframe

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const DefaultSnapshotLength = 65535

// Config 描述平台原生帧 driver 的通用配置。
type Config struct {
	InterfaceName  string
	InterfaceIndex int
	EtherType      uint16
	Promiscuous    bool
	SnapshotLength int
	Immediate      bool
	ReadTimeout    time.Duration
	BufferSize     int
}

// Meta 描述平台 driver 解析出的二层帧元数据。
type Meta struct {
	InterfaceName  string
	InterfaceIndex int
	Source         net.HardwareAddr
	Destination    net.HardwareAddr
	EtherType      uint16
}

// Frame 保存平台 driver 读取到的一帧完整链路层数据。
type Frame struct {
	Meta Meta
	Data []byte
}

// Endpoint 是平台原生帧收发端的最小接口。
type Endpoint interface {
	Addr() string
	ReadFrame(ctx context.Context, readBufferSize int) (Frame, error)
	WriteFrame(ctx context.Context, data []byte) error
	Close() error
}

func normalizeSnapshotLength(value int) (int, error) {
	if value == 0 {
		return DefaultSnapshotLength, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("%w: negative snapshot length", ErrInvalidConfig)
	}
	return value, nil
}

func parseEthernetMeta(name string, index int, data []byte) Meta {
	meta := Meta{InterfaceName: name, InterfaceIndex: index}
	if len(data) >= 14 {
		meta.Destination = append(net.HardwareAddr(nil), data[0:6]...)
		meta.Source = append(net.HardwareAddr(nil), data[6:12]...)
		meta.EtherType = binary.BigEndian.Uint16(data[12:14])
	}
	return meta
}

func resolveInterface(name string, index int) (*net.Interface, error) {
	if name != "" {
		return net.InterfaceByName(name)
	}
	if index > 0 {
		return net.InterfaceByIndex(index)
	}
	return nil, fmt.Errorf("%w: missing interface", ErrInvalidConfig)
}

func matchEtherType(data []byte, etherType uint16) bool {
	return etherType == 0 || len(data) >= 14 && binary.BigEndian.Uint16(data[12:14]) == etherType
}
