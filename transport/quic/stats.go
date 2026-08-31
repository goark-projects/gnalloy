package quic

import "goark.dev/gnalloy/transport/udp"

// RuntimeStats 是低层 QUIC packet engine 的轻量状态快照。
type RuntimeStats struct {
	// State 是连接骨架当前生命周期状态。
	State ConnectionState
	// CongestionWindow 是当前拥塞窗口字节数。
	CongestionWindow int
	// CongestionInFlight 是拥塞控制器中的 bytes_in_flight。
	CongestionInFlight int
	// ActiveStreams 是当前 runtime 跟踪的 stream 数。
	ActiveStreams int
	// InFlightPacketsBySpace 是各 packet number space 中尚未完成判定的包数。
	InFlightPacketsBySpace map[PacketNumberSpace]int
	// ActivePath 是当前有效的 UDP 远端路径。
	ActivePath udp.Address
}

// Stats 返回 runtime 当前的 ACK、拥塞、stream 和路径统计快照。
func (r *Runtime) Stats() RuntimeStats {
	if r == nil {
		return RuntimeStats{InFlightPacketsBySpace: map[PacketNumberSpace]int{}}
	}
	stats := RuntimeStats{
		InFlightPacketsBySpace: make(map[PacketNumberSpace]int, 3),
	}
	if r.conn != nil {
		stats.State = r.conn.State
	}
	if r.Congestion != nil {
		stats.CongestionWindow = r.Congestion.Window()
		stats.CongestionInFlight = r.Congestion.InFlight()
	}
	if r.Streams != nil {
		stats.ActiveStreams = r.Streams.Len()
	}
	if r.Loss != nil {
		for _, space := range []PacketNumberSpace{
			PacketNumberSpaceInitial,
			PacketNumberSpaceHandshake,
			PacketNumberSpaceApplication,
		} {
			stats.InFlightPacketsBySpace[space] = r.Loss.InFlight(space)
		}
	}
	if r.Paths != nil {
		stats.ActivePath = r.Paths.Active()
	}
	return stats
}
