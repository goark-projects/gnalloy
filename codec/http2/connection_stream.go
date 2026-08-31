package http2

func (c *ConnectionController) applyRemoteInitialWindowDelta(delta int32) error {
	if delta == 0 {
		return nil
	}
	for _, stream := range c.streams {
		next := int64(stream.sendWindow) + int64(delta)
		if next > int64(maxStreamID) || next < -int64(maxStreamID) {
			return ErrFlowControl
		}
		stream.sendWindow = int32(next)
	}
	return nil
}

func (c *ConnectionController) stream(id StreamID, local bool) (*multiplexedStream, bool, error) {
	if !id.Valid() {
		return nil, false, ErrInvalidStreamID
	}
	if stream := c.streams[id]; stream != nil {
		return stream, false, nil
	}
	if !c.validInitiator(id, local) {
		return nil, false, ErrInvalidStreamID
	}
	if !local && c.cfg.MaxConcurrentStreams > 0 && len(c.streams) >= c.cfg.MaxConcurrentStreams {
		return nil, false, ErrInvalidStreamState
	}
	stream := &multiplexedStream{
		id:         id,
		state:      StreamIdle,
		sendWindow: c.remoteSettings.InitialWindowSize,
		recvWindow: c.initialStreamWindow,
	}
	c.streams[id] = stream
	return stream, true, nil
}

func (c *ConnectionController) validInitiator(id StreamID, local bool) bool {
	if local {
		if c.cfg.Server {
			return id.ServerInitiated()
		}
		return id.ClientInitiated()
	}
	if c.cfg.Server {
		return id.ClientInitiated()
	}
	return id.ServerInitiated()
}

func (c *ConnectionController) closeIfNeeded(stream *multiplexedStream) {
	if stream.state == StreamClosed {
		delete(c.streams, stream.id)
	}
}

func (c *ConnectionController) closeStream(id StreamID) {
	delete(c.streams, id)
}

func (c *ConnectionController) closeStreamsAfter(last StreamID) {
	for id := range c.streams {
		if id > last {
			delete(c.streams, id)
		}
	}
}

func canReceiveData(state StreamState) bool {
	return state == StreamOpen || state == StreamHalfClosedLocal
}

func addReceiveWindow(window *int32, increment uint32) error {
	if !addWindow(window, increment) {
		return ErrFlowControl
	}
	return nil
}
