# gnalloy Benchmark Parity Guide

本文档定义 gnalloy 与 Netty、gnet、netpoll 对比时必须遵守的性能验证口径。
任何“超过 Netty”的结论都必须附带同机、同协议、同 payload、同连接模型的数据。

## 必测维度

| 维度 | 要求 |
| --- | --- |
| 平台 | Linux、macOS、Windows 分开记录，不允许跨平台混写结论。 |
| 后端 | `epoll`、`kqueue`、`io_uring`、`iocp`、`std` 分别记录。 |
| 协议 | TCP echo、length-field、HTTP/1、HTTP/2 stream、UDP、QUIC packet/runtime。 |
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

## 对外对比口径

- Netty 必须固定 JVM 版本、GC、`EventLoopGroup`、allocator、native transport 和 handler pipeline。
- gnet/netpoll 必须固定 Go 版本、GOMAXPROCS、poller 后端、payload 和连接数。
- 每轮对比前必须预热，正式采样至少三轮，报告中保留原始命令和机器信息。
- 只允许按场景下结论，例如 “Linux/io_uring/TCP echo/1 KiB payload/P99 latency 优于 Netty epoll”，不能写泛化结论。
- 回归脚本只证明 gnalloy 自身未退化，不等于外部框架对比胜出。

## 当前边界

- `buffer.PooledAllocator` 提供跨平台 size-class 池化能力；Linux 固定块 off-heap 场景仍优先用 mmap allocator。
- `codec/http2.StreamMultiplexer` 提供 stream 生命周期和基础 flow-control，不包含 HPACK 语义压缩。
- `transport/quic.Runtime` 提供 ACK、loss、congestion、stream、path 状态基础，不包含 TLS 1.3 packet protection 和完整互通栈。
- `observability.PrometheusExporter` 是无依赖文本导出器；OpenTelemetry 应作为独立 adapter 实现。
