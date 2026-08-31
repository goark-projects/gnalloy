package http2

const (
	// SettingHeaderTableSize 对应 SETTINGS_HEADER_TABLE_SIZE。
	SettingHeaderTableSize uint16 = 0x1
	// SettingEnablePush 对应 SETTINGS_ENABLE_PUSH。
	SettingEnablePush uint16 = 0x2
	// SettingMaxConcurrentStreams 对应 SETTINGS_MAX_CONCURRENT_STREAMS。
	SettingMaxConcurrentStreams uint16 = 0x3
	// SettingInitialWindowSize 对应 SETTINGS_INITIAL_WINDOW_SIZE。
	SettingInitialWindowSize uint16 = 0x4
	// SettingMaxFrameSize 对应 SETTINGS_MAX_FRAME_SIZE。
	SettingMaxFrameSize uint16 = 0x5
	// SettingMaxHeaderListSize 对应 SETTINGS_MAX_HEADER_LIST_SIZE。
	SettingMaxHeaderListSize uint16 = 0x6
)

const defaultHeaderTableSize uint32 = 4096

// SettingsSnapshot 保存当前已应用的 HTTP/2 SETTINGS 视图。
type SettingsSnapshot struct {
	HeaderTableSize      uint32
	EnablePush           bool
	MaxConcurrentStreams uint32
	InitialWindowSize    int32
	MaxFrameSize         int
	MaxHeaderListSize    uint32
}

func defaultSettingsSnapshot(initialWindow int32, maxStreams int, enablePush bool) SettingsSnapshot {
	return SettingsSnapshot{
		HeaderTableSize:      defaultHeaderTableSize,
		EnablePush:           enablePush,
		MaxConcurrentStreams: uint32(maxStreams),
		InitialWindowSize:    initialWindow,
		MaxFrameSize:         DefaultMaxFrameSize,
	}
}

func (s *SettingsSnapshot) apply(settings []Setting) error {
	for _, setting := range settings {
		switch setting.ID {
		case SettingHeaderTableSize:
			s.HeaderTableSize = setting.Value
		case SettingEnablePush:
			if setting.Value > 1 {
				return ErrInvalidFrame
			}
			s.EnablePush = setting.Value == 1
		case SettingMaxConcurrentStreams:
			s.MaxConcurrentStreams = setting.Value
		case SettingInitialWindowSize:
			if setting.Value > uint32(maxStreamID) {
				return ErrFlowControl
			}
			s.InitialWindowSize = int32(setting.Value)
		case SettingMaxFrameSize:
			if setting.Value < uint32(DefaultMaxFrameSize) || setting.Value > uint32(MaxFrameSizeLimit) {
				return ErrInvalidFrame
			}
			s.MaxFrameSize = int(setting.Value)
		case SettingMaxHeaderListSize:
			s.MaxHeaderListSize = setting.Value
		}
	}
	return nil
}
