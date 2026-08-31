//go:build !linux

package sctp

// detectRuntimeSupport 返回非 Linux 构建的显式 unsupported 能力边界。
func detectRuntimeSupport() RuntimeSupport {
	return RuntimeSupport{
		Platform:         runtimePlatform(),
		NativeSocket:     false,
		ReadinessPoller:  false,
		CompletionPoller: false,
		OneToOneStream:   false,
	}
}
