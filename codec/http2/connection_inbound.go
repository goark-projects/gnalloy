package http2

func (c *ConnectionController) readFrame(frame TypedFrame) error {
	switch f := frame.(type) {
	case SettingsFrame:
		return c.readSettings(f)
	case HeadersFrame:
		return c.readHeaders(f.StreamID, f.Flags&FlagEndStream != 0)
	case HeadersBlock:
		return c.readHeaders(f.StreamID, f.EndStream)
	case PushPromiseFrame:
		return c.readPushPromise(f.StreamID, f.PromisedStreamID)
	case PushPromiseBlock:
		return c.readPushPromise(f.StreamID, f.PromisedStreamID)
	case DataFrame:
		return c.readData(f)
	case RSTStreamFrame:
		c.closeStream(f.StreamID)
	case GoAwayFrame:
		c.readGoAway(f)
	}
	return nil
}

func (c *ConnectionController) readSettings(frame SettingsFrame) error {
	if frame.Ack {
		return nil
	}
	before := c.remoteSettings.InitialWindowSize
	if err := c.remoteSettings.apply(frame.Settings); err != nil {
		return err
	}
	if before != c.remoteSettings.InitialWindowSize {
		return c.applyRemoteInitialWindowDelta(c.remoteSettings.InitialWindowSize - before)
	}
	return nil
}

func (c *ConnectionController) readHeaders(streamID StreamID, endStream bool) error {
	stream, _, err := c.stream(streamID, false)
	if err != nil {
		return err
	}
	if err := stream.openRemote(endStream); err != nil {
		return err
	}
	c.closeIfNeeded(stream)
	return nil
}

func (c *ConnectionController) readPushPromise(parentID StreamID, promisedID StreamID) error {
	if !c.localSettings.EnablePush {
		return ErrInvalidFrame
	}
	if c.streams[parentID] == nil {
		return ErrInvalidStreamState
	}
	_, _, err := c.stream(promisedID, false)
	return err
}

func (c *ConnectionController) readData(frame DataFrame) error {
	stream := c.streams[frame.StreamID]
	if stream == nil || !canReceiveData(stream.state) {
		return ErrInvalidStreamState
	}
	size := readableBytes(frame.Data)
	if size > int(c.connectionReceiveWindow) || size > int(stream.recvWindow) {
		return ErrFlowControl
	}
	c.connectionReceiveWindow -= int32(size)
	stream.recvWindow -= int32(size)
	if frame.Flags&FlagEndStream != 0 {
		if err := stream.halfCloseRemote(); err != nil {
			return err
		}
	}
	c.closeIfNeeded(stream)
	return nil
}

func (c *ConnectionController) readGoAway(frame GoAwayFrame) {
	c.goAwayReceived = true
	c.goAwayLastStream = frame.LastStreamID
	c.closeStreamsAfter(frame.LastStreamID)
}
