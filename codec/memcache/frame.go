package memcache

import "goark.dev/gnalloy/buffer"

const HeaderLength = 24

const (
	MagicRequest  byte = 0x80
	MagicResponse byte = 0x81
)

type Opcode byte

const (
	OpcodeGet       Opcode = 0x00
	OpcodeSet       Opcode = 0x01
	OpcodeAdd       Opcode = 0x02
	OpcodeReplace   Opcode = 0x03
	OpcodeDelete    Opcode = 0x04
	OpcodeIncrement Opcode = 0x05
	OpcodeDecrement Opcode = 0x06
	OpcodeQuit      Opcode = 0x07
	OpcodeFlush     Opcode = 0x08
	OpcodeGetQ      Opcode = 0x09
	OpcodeNoop      Opcode = 0x0a
	OpcodeVersion   Opcode = 0x0b
	OpcodeGetK      Opcode = 0x0c
	OpcodeGetKQ     Opcode = 0x0d
	OpcodeAppend    Opcode = 0x0e
	OpcodePrepend   Opcode = 0x0f
	OpcodeStat      Opcode = 0x10
	OpcodeTouch     Opcode = 0x1c
	OpcodeGAT       Opcode = 0x1d
	OpcodeGATQ      Opcode = 0x1e
)

type Status uint16

const (
	StatusOK             Status = 0x0000
	StatusKeyNotFound    Status = 0x0001
	StatusKeyExists      Status = 0x0002
	StatusValueTooLarge  Status = 0x0003
	StatusInvalidArgs    Status = 0x0004
	StatusItemNotStored  Status = 0x0005
	StatusNonNumeric     Status = 0x0006
	StatusUnknownCommand Status = 0x0081
	StatusOutOfMemory    Status = 0x0082
)

type Frame struct {
	Magic    byte
	Opcode   Opcode
	DataType byte
	VBucket  uint16
	Status   Status
	Opaque   uint32
	CAS      uint64
	Extras   buffer.ByteBuf
	Key      buffer.ByteBuf
	Value    buffer.ByteBuf
}

func NewRequest(opcode Opcode, extras buffer.ByteBuf, key buffer.ByteBuf, value buffer.ByteBuf) Frame {
	return Frame{Magic: MagicRequest, Opcode: opcode, Extras: extras, Key: key, Value: value}
}

func NewResponse(opcode Opcode, status Status, extras buffer.ByteBuf, key buffer.ByteBuf, value buffer.ByteBuf) Frame {
	return Frame{Magic: MagicResponse, Opcode: opcode, Status: status, Extras: extras, Key: key, Value: value}
}

func (f Frame) Release() {
	if f.Extras != nil {
		f.Extras.Release()
	}
	if f.Key != nil {
		f.Key.Release()
	}
	if f.Value != nil {
		f.Value.Release()
	}
}

func (f Frame) BodyLength() int {
	return readable(f.Extras) + readable(f.Key) + readable(f.Value)
}

func (f Frame) Valid() bool {
	return (f.Magic == MagicRequest || f.Magic == MagicResponse) && f.BodyLength() <= 0xffffffff
}

func readable(buf buffer.ByteBuf) int {
	if buf == nil {
		return 0
	}
	return buf.ReadableBytes()
}
