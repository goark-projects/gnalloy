package http3

import "goark.dev/gnalloy/buffer"

const (
	DefaultMaxFrameSize = 16 * 1024 * 1024
	maxVarInt           = 1<<62 - 1
	defaultSettings     = 64
)

type FrameType uint64

const (
	FrameData                 FrameType = 0x00
	FrameHeaders              FrameType = 0x01
	FrameCancelPush           FrameType = 0x03
	FrameSettings             FrameType = 0x04
	FramePushPromise          FrameType = 0x05
	FrameGoAway               FrameType = 0x07
	FrameMaxPushID            FrameType = 0x0d
	FramePriorityUpdateStream FrameType = 0x0f
	FramePriorityUpdatePush   FrameType = 0x10
	// FrameWTStream 是 WebTransport over HTTP/3 的双向 stream 前缀帧类型。
	FrameWTStream FrameType = 0x41
)

type DataFrame struct {
	Data buffer.ByteBuf
}

func (f DataFrame) Release() {
	if f.Data != nil {
		f.Data.Release()
	}
}

type HeadersFrame struct {
	HeaderBlock buffer.ByteBuf
}

func (f HeadersFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type CancelPushFrame struct {
	PushID uint64
}

type Setting struct {
	ID    uint64
	Value uint64
}

type SettingsFrame struct {
	Settings []Setting
}

type PushPromiseFrame struct {
	PushID      uint64
	HeaderBlock buffer.ByteBuf
}

func (f PushPromiseFrame) Release() {
	if f.HeaderBlock != nil {
		f.HeaderBlock.Release()
	}
}

type GoAwayFrame struct {
	ID uint64
}

type MaxPushIDFrame struct {
	PushID uint64
}

type PriorityUpdateFrame struct {
	Type       FrameType
	ElementID  uint64
	FieldValue buffer.ByteBuf
}

func (f PriorityUpdateFrame) Release() {
	if f.FieldValue != nil {
		f.FieldValue.Release()
	}
}

type UnknownFrame struct {
	Type    FrameType
	Payload buffer.ByteBuf
}

func (f UnknownFrame) Release() {
	if f.Payload != nil {
		f.Payload.Release()
	}
}
