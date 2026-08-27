//go:build !windows

package npcap

func defaultBackend() Backend {
	return nil
}
