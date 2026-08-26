package flush

// Stats 是 flush 聚合处理器的运行时快照。
type Stats struct {
	PendingFlushes      int
	ReadInProgress      bool
	Scheduled           bool
	DownstreamFlushes   uint64
	ConsolidatedFlushes uint64
}
