//go:build !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package bpf

func defaultBackend() Backend {
	return nil
}
