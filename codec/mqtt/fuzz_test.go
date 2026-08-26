package mqtt

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxMQTTFuzzInput = 4096

func FuzzMQTTFramePipeline(f *testing.F) {
	f.Add([]byte{0xc0, 0x00})
	f.Add([]byte{0xd0, 0x00})
	f.Add([]byte{0x30, 0x03, 'a', 'b', 'c'})
	f.Add([]byte{0x10, 0x0c, 0, 4, 'M', 'Q', 'T', 'T', 4, 2, 0, 30, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxMQTTFuzzInput {
			return
		}
		frameDecoder, err := NewFrameDecoder(2048)
		if err != nil {
			t.Fatal(err)
		}
		ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
		if err := ch.Pipeline().AddLast("frame", frameDecoder); err != nil {
			t.Fatal(err)
		}
		if err := ch.Pipeline().AddLast("typed", NewTypedFrameDecoder()); err != nil {
			t.Fatal(err)
		}
		if err := ch.Pipeline().AddLast("packet", NewPacketDecoder()); err != nil {
			t.Fatal(err)
		}
		fireMQTTFuzzBytes(ch, data)
		ch.Pipeline().FireChannelInactive()
	})
}

func FuzzMQTTDecodePacket(f *testing.F) {
	f.Add([]byte{0xc0})
	f.Add([]byte{0x30, 0, 1, 'a', 'b', 'c'})
	f.Add([]byte{0x10, 0, 4, 'M', 'Q', 'T', 'T', 4, 2, 0, 30, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > maxMQTTFuzzInput {
			return
		}
		payload := mqttFuzzByteBuf(data[1:])
		frame := Frame{TypeFlags: data[0], Payload: payload}
		packet, _ := DecodePacket(frame)
		releaseMQTTFuzzValue(packet)
		frame.Release()
	})
}

func fireMQTTFuzzBytes(ch channel.Channel, data []byte) {
	if len(data) == 0 {
		ch.Pipeline().FireChannelRead(mqttFuzzByteBuf(nil))
		return
	}
	mid := len(data) / 2
	if mid > 0 {
		ch.Pipeline().FireChannelRead(mqttFuzzByteBuf(data[:mid]))
	}
	ch.Pipeline().FireChannelRead(mqttFuzzByteBuf(data[mid:]))
}

func mqttFuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

func releaseMQTTFuzzValue(value any) {
	if releasable, ok := value.(interface{ Release() }); ok {
		releasable.Release()
	}
}
