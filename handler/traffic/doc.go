// Package traffic 提供 Netty TrafficShapingHandler 风格的流量整形基础。
//
// Controller 持有可共享的读写限速器，用于实现全局限速；Handler 持有
// 单 Channel 的出站等待队列和局部统计，用于保证同一 Channel 内写入
// 顺序与 flush 语义。该包不侵入 transport 热路径，所有延迟动作都通过
// Channel 绑定的 EventLoop 时间轮调度。
package traffic
