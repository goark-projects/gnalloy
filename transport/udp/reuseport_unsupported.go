//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package udp

func reusePortSupported() bool {
	return false
}
