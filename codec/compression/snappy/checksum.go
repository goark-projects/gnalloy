package snappy

import (
	"encoding/binary"
	"hash/crc32"
	"math/bits"
)

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

func maskedChecksum(data []byte) uint32 {
	crc := crc32.Checksum(data, crc32cTable)
	return bits.RotateLeft32(crc, -15) + 0xa282ead8
}

func appendChecksum(dst []byte, data []byte) []byte {
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], maskedChecksum(data))
	return append(dst, scratch[:]...)
}

func appendLittleMedium(dst []byte, value int) []byte {
	return append(dst, byte(value), byte(value>>8), byte(value>>16))
}

func readLittleMedium(src []byte) int {
	return int(src[0]) | int(src[1])<<8 | int(src[2])<<16
}

func readLittleUint32(src []byte) uint32 {
	return binary.LittleEndian.Uint32(src)
}
