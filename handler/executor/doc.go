// Package executor 提供业务 Handler 执行器。
//
// 该包用于把耗时业务逻辑从 I/O EventLoop 中移出，职责对齐 Netty
// DefaultEventExecutorGroup 的核心使用场景：I/O 线程只负责网络事件，
// 业务 Handler 通过固定 worker 和有界队列执行。InboundHandler 会为
// 每个 Handler 实例绑定一个 worker，保证同一 Handler 的入站事件按
// 提交顺序串行执行。
package executor
