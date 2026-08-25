package spdy

import "goark.dev/gnalloy/buffer"

const (
	Version3         uint16 = 3
	headerSize              = 8
	maxPayloadLength        = 0x00ffffff
	defaultSettings         = 64
)

type FrameType uint16

const (
	FrameTypeData         FrameType = 0
	FrameTypeSynStream    FrameType = 1
	FrameTypeSynReply     FrameType = 2
	FrameTypeRSTStream    FrameType = 3
	FrameTypeSettings     FrameType = 4
	FrameTypePing         FrameType = 6
	FrameTypeGoAway       FrameType = 7
	FrameTypeHeaders      FrameType = 8
	FrameTypeWindowUpdate FrameType = 9
)

const (
	FlagFIN            byte = 0x01
	FlagUnidirectional byte = 0x02
	FlagSettingsClear  byte = 0x01
	FlagPersistValue   byte = 0x01
	FlagPersisted      byte = 0x02
)

type DataFrame struct {
	StreamID uint32
	Flags    byte
	Data     buffer.ByteBuf
}

func (f DataFrame) Last() bool {
	return f.Flags&FlagFIN != 0
}

func (f DataFrame) Release() {
	if f.Data != nil {
		f.Data.Release()
	}
}

type SynStreamFrame struct {
	StreamID             uint32
	AssociatedToStreamID uint32
	Priority             byte
	Flags                byte
	HeaderBlock          buffer.ByteBuf
}

func (f SynStreamFrame) Last() bool {
	return f.Flags&FlagFIN != 0
}

func (f SynStreamFrame) Unidirectional() bool {
	return f.Flags&FlagUnidirectional != 0
}

func (f SynStreamFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type SynReplyFrame struct {
	StreamID    uint32
	Flags       byte
	HeaderBlock buffer.ByteBuf
}

func (f SynReplyFrame) Last() bool {
	return f.Flags&FlagFIN != 0
}

func (f SynReplyFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type RSTStreamFrame struct {
	StreamID   uint32
	StatusCode uint32
}

type Setting struct {
	ID    uint32
	Value uint32
	Flags byte
}

func (s Setting) PersistValue() bool {
	return s.Flags&FlagPersistValue != 0
}

func (s Setting) Persisted() bool {
	return s.Flags&FlagPersisted != 0
}

type SettingsFrame struct {
	Flags    byte
	Settings []Setting
}

func (f SettingsFrame) ClearPreviouslyPersisted() bool {
	return f.Flags&FlagSettingsClear != 0
}

type PingFrame struct {
	ID uint32
}

type GoAwayFrame struct {
	LastGoodStreamID uint32
	StatusCode       uint32
}

type HeadersFrame struct {
	StreamID    uint32
	Flags       byte
	HeaderBlock buffer.ByteBuf
}

func (f HeadersFrame) Last() bool {
	return f.Flags&FlagFIN != 0
}

func (f HeadersFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type WindowUpdateFrame struct {
	StreamID        uint32
	DeltaWindowSize uint32
}

type UnknownFrame struct {
	Version uint16
	Type    FrameType
	Flags   byte
	Payload buffer.ByteBuf
}

func (f UnknownFrame) Release() {
	if f.Payload != nil {
		f.Payload.Release()
	}
}
