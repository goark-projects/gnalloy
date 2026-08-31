//go:build linux

package sctp

// detectRuntimeSupport 返回 Linux SCTP socket 的构建期能力边界。
func detectRuntimeSupport() RuntimeSupport {
	return RuntimeSupport{
		Platform:         runtimePlatform(),
		NativeSocket:     true,
		ReadinessPoller:  true,
		CompletionPoller: false,
		OneToOneStream:   true,
	}
}
