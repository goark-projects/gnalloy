package quic

import (
	"time"

	nativequic "github.com/quic-go/quic-go"
)

// ConnectionStats 是不暴露底层实现类型的 QUIC 连接统计快照。
type ConnectionStats struct {
	// MinRTT 是当前活动路径观测到的最小 RTT。
	MinRTT time.Duration
	// LatestRTT 是最新一次 RTT 采样。
	LatestRTT time.Duration
	// SmoothedRTT 是 RFC 9002 语义下的平滑 RTT。
	SmoothedRTT time.Duration
	// MeanDeviation 是 RTT 均值偏差估计。
	MeanDeviation time.Duration
	// BytesSent 是 QUIC 层发出的字节数，包含重传，不含 UDP 外层封装。
	BytesSent uint64
	// PacketsSent 是 QUIC 层发出的包数，包含后续判定为丢失的包。
	PacketsSent uint64
	// BytesReceived 是 QUIC 层收到的总字节数，包含重复数据。
	BytesReceived uint64
	// PacketsReceived 是 QUIC 层收到的总包数，包含无法处理的包。
	PacketsReceived uint64
	// BytesLost 是当前判定丢失的 QUIC 字节数，可能因迟到包而下降。
	BytesLost uint64
	// PacketsLost 是当前判定丢失的 QUIC 包数，可能因迟到包而下降。
	PacketsLost uint64
}

func statsFromNative(stats nativequic.ConnectionStats) ConnectionStats {
	return ConnectionStats{
		MinRTT:          stats.MinRTT,
		LatestRTT:       stats.LatestRTT,
		SmoothedRTT:     stats.SmoothedRTT,
		MeanDeviation:   stats.MeanDeviation,
		BytesSent:       stats.BytesSent,
		PacketsSent:     stats.PacketsSent,
		BytesReceived:   stats.BytesReceived,
		PacketsReceived: stats.PacketsReceived,
		BytesLost:       stats.BytesLost,
		PacketsLost:     stats.PacketsLost,
	}
}
