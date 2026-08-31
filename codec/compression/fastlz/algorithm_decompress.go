package fastlz

func decompressBlock(dst []byte, src []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	level := int(src[0]>>5) + 1
	if level != int(Level1) && level != int(Level2) {
		return 0, ErrCorruptFrame
	}
	inputIndex := 0
	outputIndex := 0
	ctrl := int(src[inputIndex]) & 31
	inputIndex++
	for {
		ref := outputIndex
		length := ctrl >> 5
		ofs := (ctrl & 31) << 8
		if ctrl >= 32 {
			length--
			ref -= ofs
			if length == 6 {
				if level == int(Level1) {
					if inputIndex >= len(src) {
						return 0, ErrCorruptFrame
					}
					length += int(src[inputIndex])
					inputIndex++
				} else {
					for {
						if inputIndex >= len(src) {
							return 0, ErrCorruptFrame
						}
						code := int(src[inputIndex])
						inputIndex++
						length += code
						if code != 255 {
							break
						}
					}
				}
			}
			if inputIndex >= len(src) {
				return 0, ErrCorruptFrame
			}
			code := int(src[inputIndex])
			inputIndex++
			ref -= code
			if level == int(Level2) && code == 255 && ofs == 31<<8 {
				if inputIndex+2 > len(src) {
					return 0, ErrCorruptFrame
				}
				far := int(src[inputIndex])<<8 | int(src[inputIndex+1])
				inputIndex += 2
				ref = outputIndex - far - fastMaxDistance
			}
			if outputIndex+length+3 > len(dst) || ref-1 < 0 {
				return 0, ErrCorruptFrame
			}
			hasMore := inputIndex < len(src)
			if hasMore {
				ctrl = int(src[inputIndex])
				inputIndex++
			}
			if ref == outputIndex {
				b := dst[ref-1]
				dst[outputIndex] = b
				dst[outputIndex+1] = b
				dst[outputIndex+2] = b
				outputIndex += 3
				for ; length > 0; length-- {
					dst[outputIndex] = b
					outputIndex++
				}
			} else {
				ref--
				copyLength := length + 3
				for i := 0; i < copyLength; i++ {
					dst[outputIndex+i] = dst[ref+i]
				}
				outputIndex += copyLength
			}
			if !hasMore {
				break
			}
			continue
		}
		length = ctrl + 1
		if outputIndex+length > len(dst) || inputIndex+length > len(src) {
			return 0, ErrCorruptFrame
		}
		copy(dst[outputIndex:outputIndex+length], src[inputIndex:inputIndex+length])
		outputIndex += length
		inputIndex += length
		if inputIndex >= len(src) {
			break
		}
		ctrl = int(src[inputIndex])
		inputIndex++
	}
	return outputIndex, nil
}
