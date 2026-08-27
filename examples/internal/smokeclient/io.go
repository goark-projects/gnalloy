package smokeclient

import "io"

func writeAll(w io.Writer, src []byte) error {
	for len(src) > 0 {
		n, err := w.Write(src)
		if n > 0 {
			src = src[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
