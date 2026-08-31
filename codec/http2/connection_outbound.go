package http2

func (c *ConnectionController) writeFrame(frame TypedFrame) error {
	switch f := frame.(type) {
	case HeadersFrame:
		return c.writeHeaders(f.StreamID, f.Flags&FlagEndStream != 0)
	case HeadersBlock:
		return c.writeHeaders(f.StreamID, f.EndStream)
	case PushPromiseFrame:
		return c.writePushPromise(f.StreamID, f.PromisedStreamID)
	case PushPromiseBlock:
		return c.writePushPromise(f.StreamID, f.PromisedStreamID)
	case DataFrame:
		return c.writeData(f)
	case RSTStreamFrame:
		c.closeStream(f.StreamID)
	case WindowUpdateFrame:
		return c.applyReceiveWindowUpdate(f)
	case GoAwayFrame:
		c.closeStreamsAfter(f.LastStreamID)
	}
	return nil
}

func (c *ConnectionController) writeHeaders(streamID StreamID, endStream bool) error {
	if c.goAwayReceived && streamID > c.goAwayLastStream {
		return ErrInvalidStreamState
	}
	stream, _, err := c.stream(streamID, true)
	if err != nil {
		return err
	}
	if err := stream.openLocal(endStream); err != nil {
		return err
	}
	c.closeIfNeeded(stream)
	return nil
}

func (c *ConnectionController) writePushPromise(parentID StreamID, promisedID StreamID) error {
	if !c.cfg.Server || !c.remoteSettings.EnablePush {
		return ErrInvalidFrame
	}
	if c.streams[parentID] == nil {
		return ErrInvalidStreamState
	}
	_, _, err := c.stream(promisedID, true)
	return err
}

func (c *ConnectionController) writeData(frame DataFrame) error {
	stream := c.streams[frame.StreamID]
	if stream == nil || !stream.canWriteData() {
		return ErrInvalidStreamState
	}
	if frame.Flags&FlagEndStream != 0 {
		if err := stream.halfCloseLocal(); err != nil {
			return err
		}
		c.closeIfNeeded(stream)
	}
	return nil
}

func (c *ConnectionController) applyReceiveWindowUpdate(frame WindowUpdateFrame) error {
	if frame.StreamID == 0 {
		return c.addConnectionReceiveWindow(frame.Increment)
	}
	stream := c.streams[frame.StreamID]
	if stream == nil {
		return ErrInvalidStreamState
	}
	return addReceiveWindow(&stream.recvWindow, frame.Increment)
}

func (c *ConnectionController) addConnectionReceiveWindow(increment uint32) error {
	if !addWindow(&c.connectionReceiveWindow, increment) {
		return ErrFlowControl
	}
	return nil
}
