package http3

func (m *StateManager) writeState(msg any) error {
	switch frame := msg.(type) {
	case SettingsFrame:
		return m.writeSettings(frame)
	case *SettingsFrame:
		if frame == nil {
			return nil
		}
		return m.writeSettings(*frame)
	case GoAwayFrame:
		return m.writeGoAway(frame)
	case *GoAwayFrame:
		if frame == nil {
			return nil
		}
		return m.writeGoAway(*frame)
	case MaxPushIDFrame:
		return m.writeMaxPushID(frame)
	case *MaxPushIDFrame:
		if frame == nil {
			return nil
		}
		return m.writeMaxPushID(*frame)
	case CancelPushFrame:
		return m.writeCancelPush(frame)
	case *CancelPushFrame:
		if frame == nil {
			return nil
		}
		return m.writeCancelPush(*frame)
	case PushPromiseFrame:
		return m.writePushPromise(frame.PushID)
	case *PushPromiseFrame:
		if frame == nil {
			return nil
		}
		return m.writePushPromise(frame.PushID)
	case PushPromiseBlock:
		return m.writePushPromise(frame.PushID)
	case *PushPromiseBlock:
		if frame == nil {
			return nil
		}
		return m.writePushPromise(frame.PushID)
	case PushIDFrame:
		return m.writePushID(frame.PushID)
	case *PushIDFrame:
		if frame == nil {
			return nil
		}
		return m.writePushID(frame.PushID)
	case PriorityUpdateFrame:
		return m.writePriorityUpdate(frame)
	case *PriorityUpdateFrame:
		if frame == nil {
			return nil
		}
		return m.writePriorityUpdate(*frame)
	default:
		return nil
	}
}

func (m *StateManager) writeSettings(frame SettingsFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seenLocalSettings {
		return ErrInvalidFrameOrder
	}
	if err := validateSettings(frame.Settings); err != nil {
		return err
	}
	m.localSettings = cloneSettings(frame.Settings)
	m.seenLocalSettings = true
	return nil
}

func (m *StateManager) writeGoAway(frame GoAwayFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if frame.ID > maxVarInt {
		return ErrInvalidVarInt
	}
	if m.outboundGoAwaySet && frame.ID > m.outboundGoAwayID {
		return ErrInvalidFrame
	}
	m.outboundGoAwaySet = true
	m.outboundGoAwayID = frame.ID
	return nil
}

func (m *StateManager) writeMaxPushID(frame MaxPushIDFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if frame.PushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if m.cfg.Server {
		return ErrInvalidFrame
	}
	if frame.PushID < m.localMaxPushID {
		return ErrInvalidFrame
	}
	m.localMaxPushID = frame.PushID
	return nil
}

func (m *StateManager) writeCancelPush(frame CancelPushFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if frame.PushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if !m.validPushForEndpoint(frame.PushID) {
		return ErrInvalidFrame
	}
	m.markPush(frame.PushID, PushStateCanceled)
	return nil
}

func (m *StateManager) writePushPromise(pushID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if !m.cfg.Server || !m.validRemotePush(pushID) || m.pushCanceled(pushID) {
		return ErrInvalidFrame
	}
	m.markPush(pushID, PushStatePromised)
	return nil
}

func (m *StateManager) writePushID(pushID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if !m.cfg.Server || !m.validRemotePush(pushID) || m.pushCanceled(pushID) {
		return ErrInvalidFrame
	}
	m.markPush(pushID, PushStatePromised)
	return nil
}

func (m *StateManager) writePriorityUpdate(frame PriorityUpdateFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if frame.ElementID > maxVarInt {
		return ErrInvalidVarInt
	}
	if frame.Type == FramePriorityUpdatePush && !m.validPushForEndpoint(frame.ElementID) {
		return ErrInvalidFrame
	}
	return nil
}
