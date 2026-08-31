# gnalloy Netty 对标总览

本文档记录 gnalloy 对 Netty 的当前对齐状态。目标不是复制 Java API，而是在
Go 的显式错误、组合式接口、引用计数 `ByteBuf` 和平台原生 I/O 模型上提供一致
体验与高性能边界。

状态含义：

- `done`: 已有实现，并有单元测试、fuzz smoke、基准或平台验证入口支撑。
- `partial`: 核心路径可用，但协议完整性、跨平台运行时或高级语义仍有限制。
- `planned`: 已确认需要，但应在后续独立切片实现。
- `defer`: 不适合当前核心包直接绑定，后续扩展包或业务层承接。

## 证据等级与验证边界

Netty 对标需要同时看 API 体验、运行时事实和同机场景数据。下表是本文档使用的
证据分级，避免把“有实现”误读成“生产环境已胜出”。

| 等级 | 含义 | 可用于什么结论 |
| --- | --- | --- |
| API/test support | 已有 Go-native API、单元测试、fuzz smoke 或 dry-run 合同。 | 可以声明能力存在、接口稳定、行为有回归保护。 |
| Runtime validated | 已在目标 OS/权限/驱动环境执行 native smoke、privileged gate 或协议互通。 | 可以声明该平台运行边界已验证。 |
| External parity proven | 已在同一台机器、同协议、同 payload、同连接模型下完成 Netty/gnet/netpoll 采样，并保留原始命令和机器信息。 | 只能声明对应场景的对标结果，不能泛化到所有协议或平台。 |
| Extension/deferred | 不属于核心包，或需要外部算法、平台、厂商库承接。 | 只能声明边界清楚，不能声明核心已覆盖。 |

当前 TCP echo 外部对标已经把 Netty baseline 调整为 Linux epoll/native transport，
`benchmarks/parity/baseline.json` 作为日常低成本合同和单点基线，
`benchmarks/parity/tcp-matrix.json` 作为正式多 payload、warmup、repeat 矩阵。
任何“超过 Netty”的表述都必须引用矩阵报告中的具体平台、后端、payload、连接数、
延迟分位和错误率。

## P0-P7 完成面

| 优先级 | 范围 | 当前状态 | 验证入口 |
| --- | --- | --- | --- |
| P0 | Bootstrap/Channel option、TCP socket option、connect timeout、Future listener EventLoop 归属、读循环公平性和平台 completion 行为 | done | `go test ./bootstrap ./channel ./transport/tcp`、`go test ./channel ./transport` |
| P1 | 业务 handler executor group、流量整形、DNS resolver cache/TCP fallback/search/hosts/CNAME、Simple/Fixed/Map ChannelPool、IP filter、pcap、轻量观测延迟指标和 Prometheus 文本导出 | done | `go test ./handler/executor ./handler/traffic ./resolver/dns ./channel/pool ./handler/ipfilter ./handler/pcap ./observability ./handler/metrics` |
| P2 | LoggingHandler、FlushConsolidationHandler、协议 fuzz smoke、Netty parity 文档、性能预算 benchmark 和回归脚本 | done | `go test ./handler/logging ./handler/flush ./codec/dns ./codec/redis ./codec/http2 ./codec/http3`、`scripts/verify-regression.*` |
| P3 | RFC9000 QUIC v1 适配、TLS 1.3 packet protection、0-RTT/session resumption、HPACK、HTTP/2 child-channel、HTTP/3 QPACK/control stream、HTTP/3 transport binding、WebTransport session binding、QUIC application exchanger、DNS-over-QUIC exchanger、L2 transport 抽象、同机/外部 benchmark baseline、TLS copy reduction/native TLS 评估、EmbeddedChannel、resolver group、Unix domain socket、OpenTelemetry adapter、跨平台验证矩阵 | done | `go test ./transport/quic/rfc9000 ./transport/quic/application ./resolver/dns/quic ./transport/http3 ./transport/webtransport ./codec/http2 ./codec/http3 ./handler/tls ./channel/embedded ./resolver/dns ./transport/l2 ./transport/unix ./observability/otel ./benchmarks/parity ./validation/platformmatrix`、`scripts/verify-platform.ps1 -SkipBench`、`go test ./...` |
| P4 | Netty `FlowControlHandler` 风格入站暂停/恢复、有限队列、AutoRead 同步、溢出释放和 read-complete 合并 | done | `go test ./handler/flow` |
| P5 | Netty HTTP Cookie/Set-Cookie 编解码体验，含请求多 Cookie、响应属性、SameSite、Expires、Max-Age 和 Append 热路径 | done | `go test ./codec/http1/cookie` |
| P6 | Netty `CorsHandler` 风格 HTTP/1 CORS 策略 handler，含 Origin 匹配、预检短路、credentials 安全 wildcard 和响应头修饰 | done | `go test ./handler/cors` |
| P7 | Netty `FileRegion` 风格文件区域直接出站，TCP 默认接入 Linux/macOS `sendfile` 和 Windows `TransmitFile` writer，同时保留 fallback encoder | done | `go test ./channel ./transport/zerocopy ./transport/tcp` |

## Bootstrap 与 Channel

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ServerBootstrap` | `bootstrap.ServerBootstrap` | done | Boss/Worker `EventLoopGroup`、child handler、child options、transport factory。 |
| `Bootstrap` | `bootstrap.Dialer` | done | 客户端 dialer 支持 connect timeout 和 channel option。 |
| `ChannelOption` | `channel.ChannelOption` | done | Go 泛型类型安全选项，避免 Java 风格运行期强转。 |
| `AttributeKey/AttributeMap` | `channel.AttributeMap` | done | Channel 级轻量属性存储。 |
| `ChannelFuture` | `channel.Future` | done | 支持完成、失败、listener、deadline 等待，并可将 listener 绑定到所属 EventLoop。 |
| `ChannelGroup` | `channel.Group` | done | 批量 close/write/flush 和 group handler。 |
| `FileRegion` | `channel.FileRegion`、`channel.FileRegionWriter`、`transport/zerocopy` | done | `Unsafe` 支持直接出站 `FileRegion`，TCP 默认注入 Linux/macOS `sendfile` 和 Windows `TransmitFile` writer；非原生 region 明确返回 unsupported，保留 fallback 编码路径。 |

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
| `FlowControlHandler` | `handler/flow` | done | 支持入站 pause/resume、暂停期间有限队列、按序恢复、AutoRead 选项同步、溢出异常和确定性释放。 |
| `CorsHandler` | `handler/cors` | done | 支持 HTTP/1 Origin 匹配、预检响应、credentials 安全 wildcard、允许方法/头、暴露头和 Max-Age。 |
| `LoggingHandler` | `handler/logging` | done | 基于标准库 `slog`，记录生命周期、读写、flush、close 和异常事件，不接管消息所有权。 |
| `FlushConsolidationHandler` | `handler/flush` | done | 读循环内合并 flush，支持阈值强制下发、无读循环延迟合并和 Future 完成传播。 |
| `SslHandler` OCSP stapling | `handler/tls.OCSPConfig` | done | 支持要求 stapled OCSP 响应、握手后 `OCSPEvent`、自定义 validator 和证书 staple helper；在线 OCSP 查询仍由业务或扩展包控制，避免阻塞 I/O 热路径。 |
| `HttpProxyHandler` / `Socks5ProxyHandler` | `handler/proxy` | done | HTTP CONNECT 和 SOCKS5 CONNECT 都提供 pipeline client handler；SOCKS5 支持 no-auth 与 username/password 子协商、BIND/UDP ASSOCIATE wire helper、半包握手累积、method/reply 失败语义、成功事件和握手剩余数据透传。 |
| Netty 风格 pipeline initializer | `recipes` | done | 使用 per-channel handler factory 装配 ByteBuf echo、length-field、HTTP/1、HTTP/2、WebSocket、MQTT 和 HTTP/3 stream pipeline，避免复用有状态 codec。 |
| `RuleBasedIpFilter` | `handler/ipfilter` | done | 支持有序 allow/deny 规则、CIDR、单 IP、UDP/raw typed message 和业务 RemoteIPProvider。 |
| `PcapWriteHandler` | `handler/pcap` | done | 支持 Pipeline 级 libpcap 捕获，默认 LINKTYPE_USER0，不接管消息所有权。 |

## Buffer 与 Codec

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ByteBuf` | `buffer.ByteBuf` | done | 读写指针、引用计数、零拷贝 `Slice`、`ReadableSlices`。 |
| `CompositeByteBuf` | `buffer.CompositeByteBuf` | done | 跨 buffer 逻辑连续视图，默认不复制 payload。 |
| `ByteBufAllocator` / `PooledByteBufAllocator` | `buffer.Allocator` / `buffer.PooledAllocator` | done | heap、跨平台 size-class pooled allocator、stat allocator、Linux mmap slab allocator。 |
| 基础 codec 模板 | `codec` | done | `ByteToMessageDecoder`、message-to-message、message-to-byte 和 duplex 组合。 |
| 常用帧 decoder/encoder | `codec` | done | length-field、line、delimiter、fixed-length。 |
| 协议 codec | `codec/*` | done | HTTP/1 object/query/compression/upgrade、HTTP/2 preface/settings ACK/h2c/http1 bridge/outbound flow helper、HTTP/3 frame/lifecycle/stats、SCTP、WebSocket、MQTT、Redis、DNS、Memcache、SMTP、SOCKS、STOMP、RTSP、XML、JSON、ICMP/IP 等。 |

完整 codec 细分矩阵见 `docs/netty-codec-parity.md`。

## Resolver 与连接池

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `DnsNameResolver` cache | `resolver/dns.MemoryCache` | done | 支持正向缓存、负缓存、最小/最大 TTL 约束、缓存清理和并发安全快照。 |
| hosts/search/ndots | `resolver/dns.StaticHosts`, `Config.SearchDomains`, `Config.Ndots` | done | 静态 hosts 优先，支持相对域名搜索顺序和去重。 |
| CNAME follow | `resolver/dns` | done | A/AAAA 查询支持受限深度 CNAME 递归，避免别名链无限循环。 |
| DNS TCP fallback | `resolver/dns.TCPExchanger` | done | UDP 响应截断时可回退 TCP 查询，避免大响应被静默截断。 |
| `resolver-dns-native-macos` | `resolver/dns/native/macos` | done | darwin 上通过 `scutil --dns` 读取系统 nameserver/search-domain 快照并转换为 `resolver/dns.Config`；非 macOS 显式返回 unsupported，运行时验证必须在 macOS 主机执行。 |
| DNS-over-QUIC | `resolver/dns/quic.Exchanger` | done | 默认 ALPN `doq` 和端口 `853`，复用 QUIC stream length-prefixed application exchanger。 |
| `SimpleChannelPool` | `channel/pool.SimplePool` | done | 无总连接数限制、保留 idle 上限和生命周期回调。 |
| `FixedChannelPool` | `channel/pool.FixedPool` | done | 支持最大连接数、最大等待队列、获取超时、健康检查、生命周期回调和统计快照。 |
| `ChannelPoolMap` | `channel/pool.Map` | done | 按 endpoint/tenant 等 key 懒加载并复用子池。 |

## 应用协议装配

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| transport-neutral request/response pipeline | `protocol.ChannelExchanger`、`protocol.Server` | done | 基于 `bootstrap.Dialer`/`ServerBootstrap` 和 Adapter，把 stream、datagram、raw packet、L2 frame 都统一成上层 request-response 体验，同时保留各 transport 的消息类型和服务端响应元数据。 |

## Transport 与 EventLoop

| Netty 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `EventLoop` | `transport.EventLoop` | done | 单 owner loop 模型，跨线程操作提交到 owner loop。 |
| `NioEventLoopGroup` | `transport.EventLoopGroup` + `epoll/kqueue/std` | done | readiness/Reactor 后端。 |
| native epoll/kqueue | `transport/poller/epoll`, `transport/poller/kqueue` | done | 平台原生 readiness backend。 |
| io_uring/IOCP completion | `transport/poller/iouring`, `transport/poller/iocp` | done | accept/read/write/close 与 datagram completion 路径。 |
| TCP/UDP/raw transport | `transport/tcp`, `transport/udp`, `transport/raw` | done | 原生 socket 生命周期、回压水位线和 platform helper。 |
| SCTP transport | `transport/sctp` | partial | Linux one-to-one SCTP stream socket 已接入 `ServerBootstrap`/`Dialer`；依赖内核 SCTP 支持，非 Linux 和 completion poller 明确返回 unsupported。 |
| L2 frame transport | `transport/l2`、`transport/l2/bpf`、`transport/l2/npcap` | done | 和 TCP/UDP/raw/QUIC 一致接入 `ServerBootstrap`/`Dialer`；Linux AF_PACKET native，macOS/BSD BPF native，Windows Npcap native；BPF/Npcap 仍保留可注入 Driver 以便测试和定制部署。 |
| QUIC packet/runtime | `transport/quic` | done | 提供 UDP 上的 QUIC packet/header/frame、连接 ID 路由、ACK tracking、packet-threshold loss recovery、Reno 风格 congestion、stream flow-control 和 path validation/migration 基础。 |
| QUIC RFC9000 connection stack | `transport/quic/rfc9000` | done | 通过生产级 QUIC 实现承接 RFC 9000 QUIC v1、TLS 1.3 packet protection、ALPN、双向 stream、单向 stream、datagram、localhost 互通和显式启用的外部互通测试。 |
| QUIC application assembly | `transport/quic/application` | done | 提供 stream request/response 和 datagram request/response 装配，当前覆盖 length-prefixed stream codec 和 datagram matcher。 |
| HTTP/3 transport binding | `transport/http3` | done | 把 RFC9000 QUIC request、control、QPACK stream 绑定为 gnalloy `Channel` pipeline，复用 `codec/http3` 的 frame/header/control/QPACK 初始化器，并提供 stream 生命周期和读写字节 session 快照。 |
| WebTransport over HTTP/3 | `transport/webtransport` + `codec/http3` | done | 提供 SETTINGS/extended CONNECT helper、CONNECT stream session ID、WT_STREAM/单向 stream 前缀、HTTP Datagram Quarter Stream ID 映射和 QUIC datagram/reset capability 校验。 |

完整 transport 边界见 `docs/transport-completion-matrix.md`。

## 观测与验证

| 能力 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| Channel 指标契约 | `observability.ChannelRecorder` | done | 供应商无关、低基数、并发安全接口。 |
| 本地聚合指标 | `observability.AtomicChannelRecorder` | done | 原子聚合器，适合 smoke、压测和嵌入式导出。 |
| Pipeline 指标 handler | `handler/metrics.ChannelMetricsHandler` | done | 记录生命周期、读写字节、flush、close 和异常。 |
| Prometheus 文本导出 | `observability.PrometheusExporter` | done | 无外部依赖导出低基数聚合指标。 |
| OpenTelemetry adapter | `observability/otel` | done | OTel 以独立 adapter 接入，核心 recorder 契约仍保持轻量。 |
| 协议 fuzz smoke | `Fuzz*` tests | done | 覆盖 length-field、line、delimiter、HTTP/1、WebSocket、MQTT、DNS、Redis、HTTP/2、HTTP/3、QUIC header/frame。 |
| 回归脚本 | `scripts/verify-regression.*` | done | 全量测试、代表性 fuzz smoke、热路径基准和平台专项检查。 |
| Netty microbench 风格套件 | `benchmarks/microbench`、`cmd/gnalloy-benchdiff -suite` | done | 维护稳定 suite 目录，支持 `hotpath` 和 `native-io` 场景展开，并复用 benchdiff 输出上一版本/当前工作树的同机中位数对比。 |
| 外部对标基线 | `benchmarks/parity/baseline.json`、`examples/parity-bench` | done | 默认可 dry-run；Netty、gnet、netpoll 外部 harness 默认 skip，安装后打开并保留原始命令、机器信息和 benchmark metrics。 |
| 跨平台门禁矩阵 | `scripts/platform-matrix.json`、`validation/platformmatrix`、`.github/workflows/validation.yml` | done | Windows/Linux/macOS 目标、readiness/completion 后端、native tests、cross-compile、source scan 和 skipped bench 均有机器可读结果。 |

## 后续独立扩展边界

| 范围 | 状态 | 原因 |
| --- | --- | --- |
| HTTP multipart 高级工具 | `codec/http1/multipart` done | 提供 Content-Type boundary 提取、受限流式解码、ByteBuf/Request 适配和 append-style form-data 编码；part 数量、头部、单 part 和总正文都有预算保护。 |
| WebSocket extension compression | `codec/websocket/deflate` done | 支持 permessage-deflate 协商参数、RSV1 显式 decoder 配置、data message 压缩/解压、分片最终聚合、控制帧透传和解压膨胀预算。 |
| brotli/snappy/lz4 等压缩 codec | `codec/compression/brotli`、`codec/compression/snappy`、`codec/compression/lz4` done | 外部算法依赖隔离在独立子包；保留 ByteBuf pipeline handler、writer/reader 复用和最大解压大小限制。 |
| TLS cipher suite catalog/name conversion/config | `handler/tls` done | 基于 Go 运行时目录提供 IANA/Java/OpenSSL/hex 名称解析、insecure 显式 opt-in、TLS 1.0-1.2 配置应用和 TLS 1.3 不可配置边界。 |
| OpenSSL/native TLS、证书热更新等高级 TLS 能力 | planned | `handler/tls` 保持标准库 TLS 主路径；native TLS provider 需要平台依赖、复制预算和安全审计独立切片。 |
| true sendfile/splice 零拷贝文件传输 | `transport/zerocopy` done | Linux/macOS `sendfile` 和 Windows `TransmitFile` 可直接传输 `DefaultFileRegion` backed by `*os.File`；TCP `Unsafe` 出站默认接入该 writer，非原生 region 明确返回 unsupported，保留 `Copy` 与 `FileRegionEncoder` fallback。 |
| UDT、RXTX/serial transport | defer | 依赖平台模块或过时协议生态，适合独立 transport 扩展，不绑定核心发布节奏。 |
| in-VM local transport | defer | Go 里可用 memory backend 与嵌入式测试替代；无需复制 Netty 的 JVM 内本地传输模型。 |
| 对象序列化/marshalling | defer | Go 网络核心不应绑定 Java 风格对象序列化框架。 |
