//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package unix

func abstractSocketSupported() bool {
	return false
}
