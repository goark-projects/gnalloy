package quic

import (
	"encoding/binary"

	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/udp"
)

type ConnectionState uint8

const (
	ConnectionStateNew ConnectionState = iota + 1
	ConnectionStateActive
	ConnectionStateClosing
	ConnectionStateClosed
)

type PacketNumberSpace uint8

const (
	PacketNumberSpaceInitial PacketNumberSpace = iota + 1
	PacketNumberSpaceHandshake
	PacketNumberSpaceApplication
)

// FiveTupleKey 标识 UDP 会话路径。地址使用固定数组，避免 map key 分配。
type FiveTupleKey struct {
	Local  transport.SocketAddress
	Remote transport.SocketAddress
}

// Connection 是 QUIC packet engine 的最小连接骨架。
type Connection struct {
	Version       Version
	DestinationID ConnectionID
	SourceID      ConnectionID
	Remote        udp.Address
	State         ConnectionState

	runtime *Runtime
}

// ConnectionIDRouter 负责按 DCID 路由 QUIC packet。
type ConnectionIDRouter struct {
	limit       int
	connections map[string]*Connection
}

func NewConnectionIDRouter(activeConnectionIDLimit int) *ConnectionIDRouter {
	if activeConnectionIDLimit < 2 {
		activeConnectionIDLimit = 2
	}
	return &ConnectionIDRouter{
		limit:       activeConnectionIDLimit,
		connections: make(map[string]*Connection, activeConnectionIDLimit),
	}
}

func (r *ConnectionIDRouter) Len() int {
	if r == nil {
		return 0
	}
	return len(r.connections)
}

func (r *ConnectionIDRouter) Add(conn *Connection) error {
	if r == nil || conn == nil || conn.DestinationID.Empty() || !conn.Version.Valid() {
		return ErrInvalidPacket
	}
	if len(r.connections) >= r.limit {
		if _, exists := r.connections[conn.DestinationID.String()]; !exists {
			return ErrInvalidConnectionID
		}
	}
	r.connections[conn.DestinationID.String()] = conn
	return nil
}

func (r *ConnectionIDRouter) Find(dcid ConnectionID) (*Connection, bool) {
	if r == nil || dcid.Empty() {
		return nil, false
	}
	conn, ok := r.connections[dcid.String()]
	return conn, ok
}

func (r *ConnectionIDRouter) Remove(dcid ConnectionID) {
	if r == nil || dcid.Empty() {
		return
	}
	delete(r.connections, dcid.String())
}

// Route 按 DCID 查找连接；Initial 首包不存在时创建新连接骨架。
func (r *ConnectionIDRouter) Route(packet Packet, remote udp.Address) (*Connection, bool, error) {
	if r == nil || !packet.Valid() {
		return nil, false, ErrInvalidPacket
	}
	if conn, ok := r.Find(packet.DestinationID); ok {
		return conn, false, nil
	}
	if packet.Type != PacketInitial {
		return nil, false, ErrConnectionNotFound
	}
	conn := &Connection{
		Version:       packet.Version,
		DestinationID: packet.DestinationID,
		SourceID:      packet.SourceID,
		Remote:        remote,
		State:         ConnectionStateNew,
	}
	if err := r.Add(conn); err != nil {
		return nil, false, err
	}
	return conn, true, nil
}

func PacketSpace(packetType PacketType) PacketNumberSpace {
	switch packetType {
	case PacketInitial:
		return PacketNumberSpaceInitial
	case PacketHandshake:
		return PacketNumberSpaceHandshake
	default:
		return PacketNumberSpaceApplication
	}
}

func MakeFiveTupleKey(local udp.Address, remote udp.Address) (FiveTupleKey, error) {
	localAddr, err := udpAddressToSocketAddress(local)
	if err != nil {
		return FiveTupleKey{}, err
	}
	remoteAddr, err := udpAddressToSocketAddress(remote)
	if err != nil {
		return FiveTupleKey{}, err
	}
	return FiveTupleKey{Local: localAddr, Remote: remoteAddr}, nil
}

// AppendVersionNegotiation 构建 RFC 9000 Version Negotiation packet。
func AppendVersionNegotiation(dst []byte, destinationID ConnectionID, sourceID ConnectionID, versions []Version) ([]byte, error) {
	if destinationID.Len() > MaxConnectionIDLength || sourceID.Len() > MaxConnectionIDLength || len(versions) == 0 {
		return nil, ErrInvalidPacket
	}
	dst = append(dst, headerFormBit|fixedBit)
	var zero [4]byte
	dst = append(dst, zero[:]...)
	dst = append(dst, byte(destinationID.Len()))
	dst = destinationID.AppendTo(dst)
	dst = append(dst, byte(sourceID.Len()))
	dst = sourceID.AppendTo(dst)
	var tmp [4]byte
	for _, version := range versions {
		if !version.Valid() {
			return nil, ErrInvalidVersion
		}
		binary.BigEndian.PutUint32(tmp[:], uint32(version))
		dst = append(dst, tmp[:]...)
	}
	return dst, nil
}

func IsVersionNegotiation(data []byte) bool {
	return len(data) >= 7 && data[0]&headerFormBit != 0 && data[0]&fixedBit != 0 && binary.BigEndian.Uint32(data[1:5]) == 0
}

func udpAddressToSocketAddress(addr udp.Address) (transport.SocketAddress, error) {
	var out transport.SocketAddress
	out.Port = addr.Port
	if ip4 := addr.IP.To4(); ip4 != nil {
		out.Family = transport.SocketFamilyIPv4
		copy(out.IP[:4], ip4)
		return out, nil
	}
	ip16 := addr.IP.To16()
	if ip16 == nil {
		return transport.SocketAddress{}, udp.ErrInvalidAddress
	}
	out.Family = transport.SocketFamilyIPv6
	copy(out.IP[:], ip16)
	return out, nil
}
