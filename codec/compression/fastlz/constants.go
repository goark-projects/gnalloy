package fastlz

const (
	magicNumber = uint32('F')<<16 | uint32('L')<<8 | uint32('Z')

	optionCompressed byte = 0x01
	optionChecksum   byte = 0x10

	maxChunkLength   = 0xffff
	minCompressBytes = 32
	minHeaderLength  = 6

	fastMaxDistance    = 8191
	fastMaxFarDistance = 65535 + fastMaxDistance - 1
	fastHashLog        = 13
	fastHashSize       = 1 << fastHashLog
	fastHashMask       = fastHashSize - 1
	fastMaxCopy        = 32
	fastMaxLen         = 256 + 8
	fastLevel2MinBytes = 1024 * 64
)
