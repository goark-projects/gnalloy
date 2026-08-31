package memcache

import "goark.dev/gnalloy/buffer"

// Request 表示完整 Memcached binary request。
type Request struct {
	Opcode   Opcode
	DataType byte
	VBucket  uint16
	Opaque   uint32
	CAS      uint64
	Extras   buffer.ByteBuf
	Key      buffer.ByteBuf
	Value    buffer.ByteBuf
}

// FullRequest 是 Netty 命名风格的完整 request 别名。
type FullRequest = Request

// Response 表示完整 Memcached binary response。
type Response struct {
	Opcode   Opcode
	DataType byte
	Status   Status
	Opaque   uint32
	CAS      uint64
	Extras   buffer.ByteBuf
	Key      buffer.ByteBuf
	Value    buffer.ByteBuf
}

// FullResponse 是 Netty 命名风格的完整 response 别名。
type FullResponse = Response

// NewFullRequest 创建完整 request 对象，调用方把 ByteBuf 所有权交给返回值。
func NewFullRequest(opcode Opcode, extras buffer.ByteBuf, key buffer.ByteBuf, value buffer.ByteBuf) Request {
	return Request{Opcode: opcode, Extras: extras, Key: key, Value: value}
}

// NewFullResponse 创建完整 response 对象，调用方把 ByteBuf 所有权交给返回值。
func NewFullResponse(opcode Opcode, status Status, extras buffer.ByteBuf, key buffer.ByteBuf, value buffer.ByteBuf) Response {
	return Response{Opcode: opcode, Status: status, Extras: extras, Key: key, Value: value}
}

// Release 释放 request 持有的 ByteBuf。
func (r Request) Release() {
	releasePart(r.Extras)
	releasePart(r.Key)
	releasePart(r.Value)
}

// BodyLength 返回完整 request body 长度。
func (r Request) BodyLength() int {
	return readable(r.Extras) + readable(r.Key) + readable(r.Value)
}

// Valid 校验 request 是否能编码为 Memcached binary frame。
func (r Request) Valid() bool {
	return r.BodyLength() <= 0xffffffff && readable(r.Extras) <= 0xff && readable(r.Key) <= 0xffff
}

func (r Request) toFrame() Frame {
	return Frame{
		Magic:    MagicRequest,
		Opcode:   r.Opcode,
		DataType: r.DataType,
		VBucket:  r.VBucket,
		Opaque:   r.Opaque,
		CAS:      r.CAS,
		Extras:   retainPart(r.Extras),
		Key:      retainPart(r.Key),
		Value:    retainPart(r.Value),
	}
}

// Release 释放 response 持有的 ByteBuf。
func (r Response) Release() {
	releasePart(r.Extras)
	releasePart(r.Key)
	releasePart(r.Value)
}

// BodyLength 返回完整 response body 长度。
func (r Response) BodyLength() int {
	return readable(r.Extras) + readable(r.Key) + readable(r.Value)
}

// Valid 校验 response 是否能编码为 Memcached binary frame。
func (r Response) Valid() bool {
	return r.BodyLength() <= 0xffffffff && readable(r.Extras) <= 0xff && readable(r.Key) <= 0xffff
}

func (r Response) toFrame() Frame {
	return Frame{
		Magic:    MagicResponse,
		Opcode:   r.Opcode,
		DataType: r.DataType,
		Status:   r.Status,
		Opaque:   r.Opaque,
		CAS:      r.CAS,
		Extras:   retainPart(r.Extras),
		Key:      retainPart(r.Key),
		Value:    retainPart(r.Value),
	}
}
