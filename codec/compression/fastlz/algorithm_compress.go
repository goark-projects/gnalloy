package fastlz

func compressBlock(dst []byte, src []byte, level Level, htab []int) (int, bool) {
	if len(src) == 0 {
		return 0, true
	}
	if len(src) < 4 {
		return compressSmall(dst, src)
	}
	if len(htab) < fastHashSize || len(dst) < maxOutputLength(len(src)) {
		return 0, false
	}
	ip := 0
	ipBound := len(src) - 2
	ipLimit := len(src) - 12
	op := 0
	copyCount := 2
	dst[op] = fastMaxCopy - 1
	op++
	dst[op] = src[ip]
	op++
	ip++
	dst[op] = src[ip]
	op++
	ip++
	for ip < ipLimit {
		ref := 0
		distance := 0
		matchLength := 3
		anchor := ip
		matched := false
		if level == Level2 && src[ip] == src[ip-1] && readU16(src, ip-1) == readU16(src, ip+1) {
			distance = 1
			ip += 3
			ref = anchor + 2
			matched = true
		}
		if !matched {
			hslot := fastHash(src, ip)
			ref = htab[hslot]
			distance = anchor - ref
			htab[hslot] = anchor
			if distance == 0 ||
				(level == Level1 && distance >= fastMaxDistance) ||
				(level == Level2 && distance >= fastMaxFarDistance) ||
				ref+2 >= len(src) ||
				src[ref] != src[ip] ||
				src[ref+1] != src[ip+1] ||
				src[ref+2] != src[ip+2] {
				dst[op] = src[anchor]
				op++
				anchor++
				ip = anchor
				copyCount++
				if copyCount == fastMaxCopy {
					copyCount = 0
					dst[op] = fastMaxCopy - 1
					op++
				}
				continue
			}
			ref += 3
			ip += 3
			if level == Level2 && distance >= fastMaxDistance {
				if ref+1 >= len(src) || ip+1 >= len(src) || src[ref] != src[ip] || src[ref+1] != src[ip+1] {
					dst[op] = src[anchor]
					op++
					anchor++
					ip = anchor
					copyCount++
					if copyCount == fastMaxCopy {
						copyCount = 0
						dst[op] = fastMaxCopy - 1
						op++
					}
					continue
				}
				ref += 2
				ip += 2
				matchLength += 2
			}
		}
		ip = anchor + matchLength
		distance--
		if distance == 0 {
			x := src[ip-1]
			for ip < ipBound {
				if ref >= len(src) || src[ref] != x {
					break
				}
				ref++
				ip++
			}
		} else {
			for ip < ipBound && ref < len(src) && src[ref] == src[ip] {
				ref++
				ip++
			}
			if ip < ipBound {
				ip++
			}
		}
		if copyCount != 0 {
			dst[op-copyCount-1] = byte(copyCount - 1)
		} else {
			op--
		}
		copyCount = 0
		ip -= 3
		matchLength = ip - anchor
		var ok bool
		op, ok = encodeMatch(dst, op, level, distance, matchLength)
		if !ok {
			return 0, false
		}
		if ip+1 >= len(src) {
			break
		}
		hslot := fastHash(src, ip)
		htab[hslot] = ip
		ip++
		hslot = fastHash(src, ip)
		htab[hslot] = ip
		ip++
		dst[op] = fastMaxCopy - 1
		op++
	}
	ipBound++
	for ip <= ipBound {
		dst[op] = src[ip]
		op++
		ip++
		copyCount++
		if copyCount == fastMaxCopy {
			copyCount = 0
			dst[op] = fastMaxCopy - 1
			op++
		}
	}
	if copyCount != 0 {
		dst[op-copyCount-1] = byte(copyCount - 1)
	} else {
		op--
	}
	if level == Level2 {
		dst[0] |= 1 << 5
	}
	return op, true
}

func compressSmall(dst []byte, src []byte) (int, bool) {
	if len(dst) < len(src)+1 {
		return 0, false
	}
	if len(src) == 0 {
		return 0, true
	}
	dst[0] = byte(len(src) - 1)
	copy(dst[1:], src)
	return len(src) + 1, true
}

func encodeMatch(dst []byte, op int, level Level, distance int, length int) (int, bool) {
	if level == Level2 {
		return encodeLevel2Match(dst, op, distance, length)
	}
	for length > fastMaxLen-2 {
		if op+3 > len(dst) {
			return 0, false
		}
		dst[op] = byte((7 << 5) + (distance >> 8))
		dst[op+1] = byte(fastMaxLen - 2 - 7 - 2)
		dst[op+2] = byte(distance)
		op += 3
		length -= fastMaxLen - 2
	}
	if length < 7 {
		if op+2 > len(dst) {
			return 0, false
		}
		dst[op] = byte((length << 5) + (distance >> 8))
		dst[op+1] = byte(distance)
		return op + 2, true
	}
	if op+3 > len(dst) {
		return 0, false
	}
	dst[op] = byte((7 << 5) + (distance >> 8))
	dst[op+1] = byte(length - 7)
	dst[op+2] = byte(distance)
	return op + 3, true
}

func encodeLevel2Match(dst []byte, op int, distance int, length int) (int, bool) {
	if distance < fastMaxDistance {
		if length < 7 {
			if op+2 > len(dst) {
				return 0, false
			}
			dst[op] = byte((length << 5) + (distance >> 8))
			dst[op+1] = byte(distance)
			return op + 2, true
		}
		if op+3 > len(dst) {
			return 0, false
		}
		dst[op] = byte((7 << 5) + (distance >> 8))
		op++
		for length -= 7; length >= 255; length -= 255 {
			dst[op] = 255
			op++
		}
		dst[op] = byte(length)
		dst[op+1] = byte(distance)
		return op + 2, true
	}
	distance -= fastMaxDistance
	if length < 7 {
		if op+4 > len(dst) {
			return 0, false
		}
		dst[op] = byte((length << 5) + 31)
		dst[op+1] = 255
		dst[op+2] = byte(distance >> 8)
		dst[op+3] = byte(distance)
		return op + 4, true
	}
	if op+5 > len(dst) {
		return 0, false
	}
	dst[op] = byte((7 << 5) + 31)
	op++
	for length -= 7; length >= 255; length -= 255 {
		dst[op] = 255
		op++
	}
	dst[op] = byte(length)
	dst[op+1] = 255
	dst[op+2] = byte(distance >> 8)
	dst[op+3] = byte(distance)
	return op + 4, true
}

func fastHash(data []byte, offset int) int {
	v := readU16(data, offset)
	v ^= readU16(data, offset+1) ^ (v >> (16 - fastHashLog))
	return v & fastHashMask
}

func readU16(data []byte, offset int) int {
	if offset+1 >= len(data) {
		return int(data[offset])
	}
	return int(data[offset+1])<<8 | int(data[offset])
}
