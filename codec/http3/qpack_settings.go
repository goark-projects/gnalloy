package http3

const (
	// SettingQPACKMaxTableCapacity 是 RFC 9204 定义的动态表最大容量 SETTINGS。
	SettingQPACKMaxTableCapacity uint64 = 0x01
	// SettingQPACKBlockedStreams 是 RFC 9204 定义的最大阻塞 stream 数 SETTINGS。
	SettingQPACKBlockedStreams uint64 = 0x07
)

// QPACKSettings 返回本端 control stream 需要声明的 QPACK SETTINGS。
func QPACKSettings(maxTableCapacity uint64, blockedStreams uint64) []Setting {
	settings := make([]Setting, 0, 2)
	if maxTableCapacity > 0 {
		settings = append(settings, Setting{ID: SettingQPACKMaxTableCapacity, Value: maxTableCapacity})
	}
	if blockedStreams > 0 {
		settings = append(settings, Setting{ID: SettingQPACKBlockedStreams, Value: blockedStreams})
	}
	return settings
}
