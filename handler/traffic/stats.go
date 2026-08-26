package traffic

import "sync/atomic"

// Snapshot 是 traffic shaping 的只读统计快照。
type Snapshot struct {
	ReadBytes         int64
	WrittenBytes      int64
	DelayedReads      int64
	DelayedWrites     int64
	PendingWrites     int64
	PendingWriteBytes int64
}

type counters struct {
	readBytes     atomic.Int64
	writtenBytes  atomic.Int64
	delayedReads  atomic.Int64
	delayedWrites atomic.Int64
}

func (c *counters) snapshot(pendingWrites int64, pendingWriteBytes int64) Snapshot {
	return Snapshot{
		ReadBytes:         c.readBytes.Load(),
		WrittenBytes:      c.writtenBytes.Load(),
		DelayedReads:      c.delayedReads.Load(),
		DelayedWrites:     c.delayedWrites.Load(),
		PendingWrites:     pendingWrites,
		PendingWriteBytes: pendingWriteBytes,
	}
}
