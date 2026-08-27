# gnalloy Benchmark Parity Guide

本文档定义 gnalloy 与 Netty、gnet、netpoll 对比时必须遵守的性能验证口径。
任何“超过 Netty”的结论都必须附带同机、同协议、同 payload、同连接模型的数据。

## 必测维度

| 维度 | 要求 |
| --- | --- |
| 平台 | Linux、macOS、Windows 分开记录，不允许跨平台混写结论。 |
| 后端 | `epoll`、`kqueue`、`io_uring`、`iocp`、`std` 分别记录。 |
| 协议 | TCP echo、length-field、HTTP/1、HTTP/2 stream、UDP、raw IP、L2 frame、QUIC packet/runtime、QUIC stream/datagram、DNS-over-QUIC。 |
| 负载 | 小包、MTU 附近包、大包、长连接、短连接、连接 churn。 |
| 指标 | throughput、P50/P95/P99/P999 latency、allocs/op、B/op、RSS、CPU、错误率。 |
| 回压 | 高/低水位线、慢消费者、写队列增长、flush 合并策略。 |
| TLS | 明文、TLS、StartTLS、SNI、证书校验分别记录。 |

## gnalloy 本地入口

```sh
BENCHTIME=1s COUNT=3 GROUPS=buffer,codec,quic,observability ./scripts/verify-bench.sh
BACKENDS=epoll,iouring WORKERS=4 MMAP=1 GROUPS=tcp ./scripts/verify-bench.sh
```

Windows:

```powershell
.\scripts\verify-bench.ps1 -Groups buffer,codec,quic,observability -Benchtime 1s -Count 3
.\scripts\verify-bench.ps1 -Backends default,iocp -Groups tcp -Workers 4 -Benchtime 1s -Count 3
```

外部对标 baseline:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json
go run ./examples/parity-bench -config benchmarks/parity/baseline.json -out parity-report.md
```

Windows:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json
go run ./examples/parity-bench -config benchmarks/parity/baseline.json -out parity-report.md
```

`benchmarks/parity/baseline.json` 默认直接保留 gnalloy 本地场景，并启用 Netty、
gnet、netpoll 外部 harness 场景。外部 harness 源码位于 `benchmarks/external`，
构建产物输出到 `benchmarks/external/bin`；该目录是本机构建产物，不提交到仓库。
Netty 使用独立 Maven 工程，gnet/netpoll 使用独立 Go module，避免把对标依赖引入
gnalloy 根 `go.mod`。

严格外部对标 gate:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/baseline.json
```

`-strict-external` 用于正式声称同机对标前的合同检查：Netty、gnet、netpoll
以及带 `external` tag 的场景不能保持 `skip=true`，展开变量后的命令入口必须
在本机可解析；`java -jar` 场景还会检查 jar 文件本身是否存在。Windows 下 Go
harness 构建为 `.exe`，baseline 仍使用无扩展名路径，由 strict gate 和 runner
按平台解析。

外部 harness 支持的最小命令合同:

```bash
java -jar benchmarks/external/bin/netty-bench.jar --protocol tcp-echo --payload 1024 --connections 256 --messages 100000
benchmarks/external/bin/gnet-bench -protocol tcp-echo -payload 1024 -connections 256 -messages 100000
benchmarks/external/bin/netpoll-bench -protocol tcp-echo -payload 1024 -connections 256 -messages 100000
```

外部 harness 输出 Go benchmark 兼容的 `Benchmark* ns/op` 行，表示固定连接数和
固定消息数下的端到端 TCP echo RTT 均值。外部进程无法使用 Go `benchmem` 的
内存口径准确统计框架内部分配，因此不伪造 `B/op` 和 `allocs/op`。

平台边界：CloudWeGo netpoll v0.7.5 的 Windows 上游实现为空，Windows 下
`netpoll-bench` 可构建并在运行时明确返回 unsupported；Linux/macOS/BSD 才能执行
真实 netpoll echo benchmark。Windows 严格 gate 只证明 harness 入口已构建、可发现，
不等于完成 netpoll 运行时性能采样。

## 对外对比口径

- Netty 必须固定 JVM 版本、GC、`EventLoopGroup`、allocator、native transport 和 handler pipeline。
- gnet/netpoll 必须固定 Go 版本、GOMAXPROCS、poller 后端、payload 和连接数。
- 每轮对比前必须预热，正式采样至少三轮，报告中保留原始命令和机器信息。
- 只允许按场景下结论，例如 “Linux/io_uring/TCP echo/1 KiB payload/P99 latency 优于 Netty epoll”，不能写泛化结论。
- 回归脚本只证明 gnalloy 自身未退化，不等于外部框架对比胜出。

## 当前边界

- `buffer.PooledAllocator` 提供跨平台 size-class 池化能力；Linux 固定块 off-heap 场景仍优先用 mmap allocator。
- `codec/http2.StreamMultiplexer` 提供 stream 生命周期、基础 flow-control 和 child-channel 体验；HPACK 语义压缩由 `codec/http2.HeaderDecoder/HeaderEncoder` 承担。
- `codec/http3` 提供 frame、QPACK、control stream 顺序校验、QUIC 单向 stream type 前缀、WebTransport SETTINGS 和 extended CONNECT helper。
- `transport/http3` 已提供 HTTP/3 request/control/QPACK stream 到 gnalloy `Channel` pipeline 的 transport binding。
- `transport/webtransport` 已提供 WebTransport session、stream prefix、HTTP Datagram Quarter Stream ID 映射和 capability 校验。
- `transport/quic.Runtime` 提供 ACK、loss、congestion、stream、path 状态和默认 runtime pipeline；`transport/quic/rfc9000` 提供 RFC9000/TLS1.3 互通连接栈。
- `transport/quic/application` 提供 QUIC stream/datagram 应用协议装配；`resolver/dns/quic` 以 DNS-over-QUIC 覆盖真实应用协议用例。
- `transport/l2` 已提供可注入 Driver 的二层帧 transport 抽象；Linux AF_PACKET、macOS/BSD BPF、Windows Npcap 均在核心库提供 native driver，运行时验证需要本机权限、接口环境变量和对应平台驱动。
- `scripts/platform-matrix.json` 是跨平台验证事实源；`scripts/verify-platform.ps1 -SkipBench -ReportPath platform-report.json` 会输出 passed/skipped/failed gate 结果。
- `observability.PrometheusExporter` 是无依赖文本导出器；OpenTelemetry 已作为 `observability/otel` adapter 接入。
