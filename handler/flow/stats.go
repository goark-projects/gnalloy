package flow

// Snapshot 是 Handler 入站流控状态的只读快照。
type Snapshot struct {
	Paused          bool
	PendingMessages int
	PendingBytes    int64
	DroppedMessages uint64
}
