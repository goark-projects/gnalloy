package smokeclient

import (
	"encoding/binary"
	"fmt"
	"io"
)

func writeFrame(w io.Writer, payload []byte) error {
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	out := make([]byte, size)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}
