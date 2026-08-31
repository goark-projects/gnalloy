package http3

func (m *StateManager) readState(msg any) error {
	switch frame := msg.(type) {
	case SettingsFrame:
		return m.readSettings(frame)
	case *SettingsFrame:
		if frame == nil {
			return nil
		}
		return m.readSettings(*frame)
	case GoAwayFrame:
		return m.readGoAway(frame)
	case *GoAwayFrame:
		if frame == nil {
			return nil
		}
		return m.readGoAway(*frame)
	case MaxPushIDFrame:
		return m.readMaxPushID(frame)
	case *MaxPushIDFrame:
		if frame == nil {
			return nil
		}
		return m.readMaxPushID(*frame)
	case CancelPushFrame:
		return m.readCancelPush(frame)
	case *CancelPushFrame:
		if frame == nil {
			return nil
		}
		return m.readCancelPush(*frame)
	case PushPromiseFrame:
		return m.readPushPromise(frame.PushID)
	case *PushPromiseFrame:
		if frame == nil {
			return nil
		}
		return m.readPushPromise(frame.PushID)
	case PushPromiseBlock:
		return m.readPushPromise(frame.PushID)
	case *PushPromiseBlock:
		if frame == nil {
			return nil
		}
		return m.readPushPromise(frame.PushID)
	case PushIDFrame:
		return m.readPushID(frame.PushID)
	case *PushIDFrame:
		if frame == nil {
			return nil
		}
		return m.readPushID(frame.PushID)
	case PriorityUpdateFrame:
		return m.readPriorityUpdate(frame)
	case *PriorityUpdateFrame:
		if frame == nil {
			return nil
		}
		return m.readPriorityUpdate(*frame)
	default:
		return nil
	}
}

func (m *StateManager) readSettings(frame SettingsFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seenRemoteSettings {
		return ErrInvalidFrameOrder
	}
	if err := validateSettings(frame.Settings); err != nil {
		return err
	}
	m.remoteSettings = cloneSettings(frame.Settings)
	m.seenRemoteSettings = true
	return nil
}

func (m *StateManager) readGoAway(frame GoAwayFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if frame.ID > maxVarInt {
		return ErrInvalidVarInt
	}
	if m.inboundGoAwaySet && frame.ID > m.inboundGoAwayID {
		return ErrInvalidFrame
	}
	m.inboundGoAwaySet = true
	m.inboundGoAwayID = frame.ID
	return nil
}

func (m *StateManager) readMaxPushID(frame MaxPushIDFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Server {
		return ErrInvalidFrame
	}
	if frame.PushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if m.remoteMaxPushIDSet && frame.PushID < m.remoteMaxPushID {
		return ErrInvalidFrame
	}
	m.remoteMaxPushID = frame.PushID
	m.remoteMaxPushIDSet = true
	return nil
}

func (m *StateManager) readCancelPush(frame CancelPushFrame) error {
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

func (m *StateManager) readPushPromise(pushID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if m.cfg.Server || !m.validLocalPush(pushID) || m.pushCanceled(pushID) {
		return ErrInvalidFrame
	}
	m.markPush(pushID, PushStatePromised)
	return nil
}

func (m *StateManager) readPushID(pushID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pushID > maxVarInt {
		return ErrInvalidVarInt
	}
	if m.cfg.Server || !m.validLocalPush(pushID) || m.pushCanceled(pushID) {
		return ErrInvalidFrame
	}
	m.markPush(pushID, PushStatePromised)
	return nil
}

func (m *StateManager) readPriorityUpdate(frame PriorityUpdateFrame) error {
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
