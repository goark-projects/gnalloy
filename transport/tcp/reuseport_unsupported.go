//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package tcp

func reusePortSupported() bool {
	return false
}
