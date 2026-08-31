package lzf

func compressBlock(dst []byte, src []byte, htab []int) (int, bool) {
	if len(src) < minBlockToCompress || len(dst) == 0 || len(htab) < lzfHashSize {
		return 0, false
	}
	inputIndex := 0
	outputIndex := 1
	lit := 0
	hval := int(src[0])<<8 | int(src[1])
	for inputIndex < len(src)-2 {
		hval = (hval << 8) | int(src[inputIndex+2])
		hslot := ((hval >> (24 - lzfHashLog)) - hval*5) & lzfHashMask
		ref := htab[hslot]
		htab[hslot] = inputIndex
		off := inputIndex - ref - 1
		if off < lzfMaxOff && ref > 0 &&
			src[ref] == src[inputIndex] &&
			src[ref+1] == src[inputIndex+1] &&
			src[ref+2] == src[inputIndex+2] {
			matchLength := 2
			maxLength := len(src) - inputIndex - matchLength
			if maxLength > lzfMaxRef {
				maxLength = lzfMaxRef
			}
			if outputIndex+4 >= len(dst) {
				return 0, false
			}
			dst[outputIndex-lit-1] = byte(lit - 1)
			if lit == 0 {
				outputIndex--
			}
			for {
				matchLength++
				if matchLength >= maxLength || src[ref+matchLength] != src[inputIndex+matchLength] {
					break
				}
			}
			matchLength -= 2
			inputIndex++
			if matchLength < 7 {
				dst[outputIndex] = byte((off >> 8) + (matchLength << 5))
				outputIndex++
			} else {
				dst[outputIndex] = byte((off >> 8) + (7 << 5))
				dst[outputIndex+1] = byte(matchLength - 7)
				outputIndex += 2
			}
			dst[outputIndex] = byte(off)
			outputIndex += 2
			lit = 0
			inputIndex += matchLength + 1
			if inputIndex >= len(src)-2 {
				break
			}
			inputIndex -= 2
			hval = int(src[inputIndex])<<8 | int(src[inputIndex+1])
			hval = (hval << 8) | int(src[inputIndex+2])
			hslot = ((hval >> (24 - lzfHashLog)) - hval*5) & lzfHashMask
			htab[hslot] = inputIndex
			inputIndex++
			hval = (hval << 8) | int(src[inputIndex+2])
			hslot = ((hval >> (24 - lzfHashLog)) - hval*5) & lzfHashMask
			htab[hslot] = inputIndex
			inputIndex++
			continue
		}
		if outputIndex >= len(dst) {
			return 0, false
		}
		lit++
		dst[outputIndex] = src[inputIndex]
		outputIndex++
		inputIndex++
		if lit == lzfMaxLit {
			dst[outputIndex-lit-1] = byte(lit - 1)
			lit = 0
			outputIndex++
		}
	}
	if outputIndex+3 >= len(dst) {
		return 0, false
	}
	for inputIndex < len(src) {
		lit++
		dst[outputIndex] = src[inputIndex]
		outputIndex++
		inputIndex++
		if lit == lzfMaxLit {
			dst[outputIndex-lit-1] = byte(lit - 1)
			lit = 0
			outputIndex++
		}
	}
	dst[outputIndex-lit-1] = byte(lit - 1)
	if lit == 0 {
		outputIndex--
	}
	return outputIndex, true
}

func decompressBlock(dst []byte, src []byte) (int, error) {
	inputIndex := 0
	outputIndex := 0
	for inputIndex < len(src) {
		ctrl := int(src[inputIndex])
		inputIndex++
		if ctrl < lzfMaxLit {
			length := ctrl + 1
			if outputIndex+length > len(dst) {
				return 0, ErrInsufficientBuffer
			}
			if inputIndex+length > len(src) {
				return 0, ErrCorruptFrame
			}
			copy(dst[outputIndex:outputIndex+length], src[inputIndex:inputIndex+length])
			inputIndex += length
			outputIndex += length
			continue
		}
		length := ctrl >> 5
		ref := outputIndex - ((ctrl & 0x1f) << 8) - 1
		if inputIndex >= len(src) {
			return 0, ErrCorruptFrame
		}
		if length == 7 {
			length += int(src[inputIndex])
			inputIndex++
			if inputIndex >= len(src) {
				return 0, ErrCorruptFrame
			}
		}
		ref -= int(src[inputIndex])
		inputIndex++
		if ref < 0 {
			return 0, ErrCorruptFrame
		}
		length += 2
		if outputIndex+length > len(dst) {
			return 0, ErrInsufficientBuffer
		}
		for i := 0; i < length; i++ {
			dst[outputIndex+i] = dst[ref+i]
		}
		outputIndex += length
	}
	return outputIndex, nil
}
