package sctp

import "goark.dev/gnalloy/buffer"

// Message 是 SCTP payload 与 stream 元数据的 Go 化表示。
type Message struct {
	ProtocolIdentifier uint32
	StreamIdentifier   uint16
	Unordered          bool
	Fragmented         bool
	Complete           bool
	Payload            buffer.ByteBuf
}

// NewMessage 创建完整 SCTP 消息。
func NewMessage(protocolIdentifier uint32, streamIdentifier uint16, payload buffer.ByteBuf) Message {
	return Message{
		ProtocolIdentifier: protocolIdentifier,
		StreamIdentifier:   streamIdentifier,
		Complete:           true,
		Payload:            payload,
	}
}

// NewFragment 创建可能需要 CompletionHandler 聚合的 SCTP 分片。
func NewFragment(protocolIdentifier uint32, streamIdentifier uint16, payload buffer.ByteBuf, complete bool) Message {
	return Message{
		ProtocolIdentifier: protocolIdentifier,
		StreamIdentifier:   streamIdentifier,
		Fragmented:         true,
		Complete:           complete,
		Payload:            payload,
	}
}

// Release 释放消息持有的 ByteBuf。
func (m Message) Release() {
	if m.Payload != nil {
		m.Payload.Release()
	}
}

func (m Message) valid() bool {
	return m.Payload != nil
}

type fragmentKey struct {
	protocol uint32
	stream   uint16
}

func keyOf(m Message) fragmentKey {
	return fragmentKey{protocol: m.ProtocolIdentifier, stream: m.StreamIdentifier}
}
