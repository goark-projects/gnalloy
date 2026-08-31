package lzf

const (
	maxChunkLength         = 0xffff
	minBlockToCompress     = 16
	rawHeaderLength        = 5
	compressedHeaderLength = 7

	blockTypeRaw        byte = 0
	blockTypeCompressed byte = 1

	lzfHashLog  = 14
	lzfHashSize = 1 << lzfHashLog
	lzfHashMask = lzfHashSize - 1
	lzfMaxLit   = 1 << 5
	lzfMaxOff   = 1 << 13
	lzfMaxRef   = (1 << 8) + (1 << 3)
)
