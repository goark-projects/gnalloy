//go:build linux

package unix

func abstractSocketSupported() bool {
	return true
}
