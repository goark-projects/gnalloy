package quic

import (
	"testing"

	"goark.dev/gnalloy/buffer"
)

const maxQUICFuzzInput = 4096

func FuzzQUICParseVarInt(f *testing.F) {
	f.Add([]byte{0x00})
	f.Add([]byte{0x40, 0x40})
	f.Add([]byte{0x80, 0, 0, 1})
	f.Add([]byte{0xc0, 0, 0, 0, 0, 0, 0, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxQUICFuzzInput {
			return
		}
		_, _, _ = ParseVarInt(data)
	})
}

func FuzzQUICParseHeaderBytes(f *testing.F) {
	f.Add([]byte{0xc1, 0, 0, 0, 1, 1, 1, 1, 2, 0, 1, 7}, uint8(0))
	f.Add([]byte{0x40, 1, 2, 3, 4, 0x7f}, uint8(4))
	f.Add([]byte{0x80}, uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, shortCIDLength uint8) {
		if len(data) > maxQUICFuzzInput {
			return
		}
		opts := HeaderParseOptions{ShortDestinationIDLength: int(shortCIDLength % (MaxConnectionIDLength + 1))}
		_, _, _ = ParseHeaderBytes(data, opts)
	})
}

func FuzzQUICFrameScanner(f *testing.F) {
	f.Add([]byte{byte(FrameTypePing)})
	f.Add([]byte{0, 0, byte(FrameTypePing)})
	f.Add([]byte{byte(FrameTypePathChallenge), 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{byte(FrameTypeACK), 1, 0, 0, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > maxQUICFuzzInput {
			return
		}
		payload := quicFuzzByteBuf(data)
		defer payload.Release()

		scanner := NewFrameScanner(payload)
		for i := 0; i < 256; i++ {
			frame, ok, err := scanner.Next()
			if err != nil || !ok {
				return
			}
			releaseFrame(frame)
		}
	})
}

func quicFuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}
