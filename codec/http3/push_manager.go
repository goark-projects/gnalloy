package http3

// PushState 描述 HTTP/3 server push 的连接级状态。
type PushState uint8

const (
	// PushStateIdle 表示该 push ID 尚未被使用。
	PushStateIdle PushState = iota
	// PushStatePromised 表示该 push ID 已经出现过 PUSH_PROMISE。
	PushStatePromised
	// PushStateCanceled 表示该 push ID 已被取消。
	PushStateCanceled
)

func (m *StateManager) validLocalPush(pushID uint64) bool {
	return pushID <= m.localMaxPushID
}

func (m *StateManager) validRemotePush(pushID uint64) bool {
	return m.remoteMaxPushIDSet && pushID <= m.remoteMaxPushID
}

func (m *StateManager) validPushForEndpoint(pushID uint64) bool {
	if m.cfg.Server {
		return m.validRemotePush(pushID)
	}
	return m.validLocalPush(pushID)
}

func (m *StateManager) markPush(pushID uint64, state PushState) {
	if m.pushes == nil {
		m.pushes = make(map[uint64]PushState, 4)
	}
	m.pushes[pushID] = state
}

func (m *StateManager) pushCanceled(pushID uint64) bool {
	if m.pushes == nil {
		return false
	}
	return m.pushes[pushID] == PushStateCanceled
}
