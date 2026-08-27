package smokeclient

import "io"

func appendLine(payload []byte) []byte {
	out := make([]byte, 0, len(payload)+1)
	out = append(out, payload...)
	out = append(out, '\n')
	return out
}

func readLine(r io.Reader) ([]byte, error) {
	out := make([]byte, 0, 64)
	var one [1]byte
	for {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return nil, err
		}
		if one[0] == '\n' {
			if len(out) > 0 && out[len(out)-1] == '\r' {
				out = out[:len(out)-1]
			}
			return out, nil
		}
		out = append(out, one[0])
	}
}
