// Package tcp 提供基于平台 socket API 的原生 TCP ServerTransport。
//
// 稳定公共面：
//   - Config 和 DefaultConfig 描述 TCP 监听、读缓冲、reuseport 和 allocator 策略。
//   - NewTransport 返回可接入 bootstrap.ServerBootstrap 的 ServerTransport。
//   - NewMmapAllocatorFactory 为每个 Worker EventLoop 创建独占 mmap allocator。
//   - Server 的 AllocatorStats 方法可在压测和运行诊断中观测 per-worker allocator 状态。
//
// Server.Close 的顺序是停止 acceptor、关闭 active child channels、等待 inactive、
// 最后关闭 per-worker allocator。该顺序用于保护 off-heap ByteBuf 不被提前 unmap。
package tcp
