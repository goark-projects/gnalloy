package webtransport

import "encoding/binary"

const maxQUICVarInt = 1<<62 - 1

func parseQUICVarInt(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, ErrInvalidDatagram
	}
	n := 1 << (data[0] >> 6)
	if len(data) < int(n) {
		return 0, 0, ErrInvalidDatagram
	}
	switch n {
	case 1:
		return uint64(data[0] & 0x3f), 1, nil
	case 2:
		return uint64(binary.BigEndian.Uint16(data[:2]) & 0x3fff), 2, nil
	case 4:
		return uint64(binary.BigEndian.Uint32(data[:4]) & 0x3fffffff), 4, nil
	case 8:
		return binary.BigEndian.Uint64(data[:8]) & maxQUICVarInt, 8, nil
	default:
		return 0, 0, ErrInvalidDatagram
	}
}

func appendQUICVarInt(dst []byte, v uint64) ([]byte, error) {
	switch {
	case v <= 63:
		return append(dst, byte(v)), nil
	case v <= 16383:
		var tmp [2]byte
		binary.BigEndian.PutUint16(tmp[:], uint16(v)|0x4000)
		return append(dst, tmp[:]...), nil
	case v <= 1073741823:
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], uint32(v)|0x80000000)
		return append(dst, tmp[:]...), nil
	case v <= maxQUICVarInt:
		var tmp [8]byte
		binary.BigEndian.PutUint64(tmp[:], v|0xc000000000000000)
		return append(dst, tmp[:]...), nil
	default:
		return nil, ErrInvalidDatagram
	}
}
