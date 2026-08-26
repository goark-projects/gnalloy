# gnalloy Netty 对标总览

本文档记录 gnalloy 对 Netty 的当前对齐状态。目标不是复制 Java API，而是在
Go 的显式错误、组合式接口、引用计数 `ByteBuf` 和平台原生 I/O 模型上提供一致
体验与高性能边界。

状态含义：

- `done`: 已有实现，并有单元测试、fuzz smoke、基准或平台验证入口支撑。
- `partial`: 核心路径可用，但协议完整性、跨平台运行时或高级语义仍有限制。
- `planned`: 已确认需要，但应在后续独立切片实现。
- `defer`: 不适合当前核心包直接绑定，后续扩展包或业务层承接。

## P0-P2 完成面

| 优先级 | 范围 | 当前状态 | 验证入口 |
| --- | --- | --- | --- |
| P0 | Bootstrap/Channel option、TCP socket option、connect timeout、Future listener EventLoop 归属、读循环公平性和平台 completion 行为 | done | `go test ./bootstrap ./channel ./transport/tcp`、`go test ./channel ./transport` |
| P1 | 业务 handler executor group、流量整形、DNS resolver cache/TCP fallback、固定 ChannelPool、轻量观测延迟指标和 transport completion 矩阵 | done | `go test ./handler/executor ./handler/traffic ./resolver/dns ./channel/pool ./observability ./handler/metrics` |
| P2 | LoggingHandler、FlushConsolidationHandler、协议 fuzz smoke、Netty parity 文档和回归脚本 | done | `go test ./handler/logging ./handler/flush ./codec/dns ./codec/redis ./codec/http2 ./codec/http3`、`scripts/verify-regression.*` |

## Bootstrap 与 Channel

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ServerBootstrap` | `bootstrap.ServerBootstrap` | done | Boss/Worker `EventLoopGroup`、child handler、child options、transport factory。 |
| `Bootstrap` | `bootstrap.Dialer` | done | 客户端 dialer 支持 connect timeout 和 channel option。 |
| `ChannelOption` | `channel.ChannelOption` | done | Go 泛型类型安全选项，避免 Java 风格运行期强转。 |
| `AttributeKey/AttributeMap` | `channel.AttributeMap` | done | Channel 级轻量属性存储。 |
| `ChannelFuture` | `channel.Future` | done | 支持完成、失败、listener、deadline 等待，并可将 listener 绑定到所属 EventLoop。 |
| `ChannelGroup` | `channel.Group` | done | 批量 close/write/flush 和 group handler。 |
| `FileRegion` | `channel.FileRegion` | done | 提供文件区域出站消息和 fallback 编码路径。 |

## Pipeline 与 Handler

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ChannelPipeline` | `channel.Pipeline` | done | `AddFirst/AddLast/AddBefore/AddAfter/Replace/Remove` 和入站/出站传播。 |
| `ChannelHandlerContext` | `channel.HandlerContext` | done | 明确向后传播入站事件、向前传播出站事件。 |
| `ChannelInboundHandler` | `channel.ChannelReadHandler` 等接口 | done | 用小接口组合替代继承层级。 |
| `ChannelOutboundHandler` | `channel.WriteHandler` 等接口 | done | 出站错误通过显式 `error` 返回。 |
| `DefaultEventExecutorGroup` | `handler/executor` | done | 固定 worker group、有限队列、按 handler 保序 offload。 |
| `IdleStateHandler`/timeout | `handler/timeout` | done | 基于时间轮，避免每连接独立 `time.Timer` 膨胀。 |
| `TrafficShapingHandler` | `handler/traffic` | done | 支持本地/共享读写限速和指标快照。 |
| `LoggingHandler` | `handler/logging` | done | 基于标准库 `slog`，记录生命周期、读写、flush、close 和异常事件，不接管消息所有权。 |
| `FlushConsolidationHandler` | `handler/flush` | done | 读循环内合并 flush，支持阈值强制下发、无读循环延迟合并和 Future 完成传播。 |

## Buffer 与 Codec

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ByteBuf` | `buffer.ByteBuf` | done | 读写指针、引用计数、零拷贝 `Slice`、`ReadableSlices`。 |
| `CompositeByteBuf` | `buffer.CompositeByteBuf` | done | 跨 buffer 逻辑连续视图，默认不复制 payload。 |
| `ByteBufAllocator` | `buffer.Allocator` | done | heap、stat allocator、Linux mmap slab allocator。 |
| 基础 codec 模板 | `codec` | done | `ByteToMessageDecoder`、message-to-message、message-to-byte 和 duplex 组合。 |
| 常用帧 decoder/encoder | `codec` | done | length-field、line、delimiter、fixed-length。 |
| 协议 codec | `codec/*` | done | HTTP/1、HTTP/2、HTTP/3 frame、WebSocket、MQTT、Redis、DNS、Memcache、SMTP、SOCKS、STOMP、RTSP、XML、JSON、ICMP/IP 等。 |

完整 codec 细分矩阵见 `docs/netty-codec-parity.md`。

## Resolver 与连接池

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `DnsNameResolver` cache | `resolver/dns.MemoryCache` | done | 支持正向缓存、负缓存、最小/最大 TTL 约束和并发安全快照。 |
| DNS TCP fallback | `resolver/dns.TCPExchanger` | done | UDP 响应截断时可回退 TCP 查询，避免大响应被静默截断。 |
| `FixedChannelPool` | `channel/pool.FixedPool` | done | 支持最大连接数、最大等待队列、获取超时、健康检查和统计快照。 |

## Transport 与 EventLoop

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `EventLoop` | `transport.EventLoop` | done | 单 owner loop 模型，跨线程操作提交到 owner loop。 |
| `NioEventLoopGroup` | `transport.EventLoopGroup` + `epoll/kqueue/std` | done | readiness/Reactor 后端。 |
| native epoll/kqueue | `transport/poller/epoll`, `transport/poller/kqueue` | done | 平台原生 readiness backend。 |
| io_uring/IOCP completion | `transport/poller/iouring`, `transport/poller/iocp` | done | accept/read/write/close 与 datagram completion 路径。 |
| TCP/UDP/raw transport | `transport/tcp`, `transport/udp`, `transport/raw` | done | 原生 socket 生命周期、回压水位线和 platform helper。 |
| QUIC | `transport/quic` | partial | 当前是 UDP 上的 QUIC packet engine，不宣称完整 TLS 1.3、拥塞控制、丢包恢复、迁移和 stream 状态机。 |

完整 transport 边界见 `docs/transport-completion-matrix.md`。

## 观测与验证

| 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| Channel 指标契约 | `observability.ChannelRecorder` | done | 供应商无关、低基数、并发安全接口。 |
| 本地聚合指标 | `observability.AtomicChannelRecorder` | done | 原子聚合器，适合 smoke、压测和嵌入式导出。 |
| Pipeline 指标 handler | `handler/metrics.ChannelMetricsHandler` | done | 记录生命周期、读写字节、flush、close 和异常。 |
| 协议 fuzz smoke | `Fuzz*` tests | done | 覆盖 length-field、line、delimiter、HTTP/1、WebSocket、MQTT、DNS、Redis、HTTP/2、HTTP/3、QUIC header/frame。 |
| 回归脚本 | `scripts/verify-regression.*` | done | 全量测试、代表性 fuzz smoke、热路径基准和平台专项检查。 |

## 仍需后续独立切片

| 范围 | 状态 | 原因 |
| --- | --- | --- |
| QUIC 完整连接协议栈 | planned | TLS 1.3、ACK/loss recovery、拥塞控制、stream multiplexing 需要独立状态机设计。 |
| OpenTelemetry exporter | planned | 当前核心只提供无依赖契约；OTel 应作为适配层接入，避免核心包强绑定外部依赖。 |
| brotli/snappy/lz4 等压缩 codec | defer | 需要外部算法依赖，适合扩展包。 |
| 对象序列化/marshalling | defer | Go 网络核心不应绑定 Java 风格对象序列化框架。 |
