// Package bpf 提供 macOS/BSD BPF 二层驱动的扩展边界。
//
// 本包只定义 Driver、Backend 和 Config 契约，不直接打开 /dev/bpf，也不引入
// cgo 或平台动态库依赖。真实 BPF 后端应由独立扩展包实现 Backend 后注入到
// transport/l2.Config.Driver。
package bpf
