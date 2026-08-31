# gnalloy Benchmark Parity Guide

本文档定义 gnalloy 与 Netty、gnet、netpoll 对比时必须遵守的性能验证口径。
任何“超过 Netty”的结论都必须附带同机、同协议、同 payload、同连接模型的数据。

## 必测维度

| 维度 | 要求 |
| --- | --- |
| 平台 | Linux、macOS、Windows 分开记录，不允许跨平台混写结论。 |
| 后端 | `epoll`、`kqueue`、`io_uring`、`iocp`、`std` 分别记录。 |
| 协议 | TCP echo、length-field、HTTP/1、HTTP/2 stream、HTTP/3 stream、UDP echo、raw IP、L2 frame、QUIC stream/datagram、DNS-over-QUIC。 |
| 负载 | 小包、MTU 附近包、大包、长连接、短连接、连接 churn。 |
| 指标 | throughput、P50/P95/P99/P999 latency、allocs/op、B/op、RSS、CPU、错误率。 |
| 回压 | 高/低水位线、慢消费者、写队列增长、flush 合并策略。 |
| TLS | 明文、TLS、StartTLS、SNI、证书校验分别记录。 |

## gnalloy 本地入口

```sh
BENCHTIME=1s COUNT=3 GROUPS=buffer,codec,observability ./scripts/verify-bench.sh
BACKENDS=epoll,iouring WORKERS=4 MMAP=1 GROUPS=tcp ./scripts/verify-bench.sh
```

Windows:

```powershell
.\scripts\verify-bench.ps1 -Groups buffer,codec,quic,observability -Benchtime 1s -Count 3
.\scripts\verify-bench.ps1 -Backends default,iocp -Groups tcp -Workers 4 -Benchtime 1s -Count 3
```

上一版本对比:

```bash
go run ./cmd/gnalloy-benchdiff -base HEAD -packages ./buffer -bench 'BenchmarkPooledAllocator' -count 5 -benchtime 500ms
go run ./cmd/gnalloy-benchdiff -base HEAD~1 -packages ./channel -bench 'BenchmarkPipelineWriteAndFlushDirectSink$' -count 5 -benchtime 500ms -out benchdiff.md
go run ./cmd/gnalloy-benchdiff -base HEAD~1 -suite hotpath -count 5 -benchtime 500ms -out hotpath-benchdiff.md
```

Windows:

```powershell
go run ./cmd/gnalloy-benchdiff -base HEAD -packages ./buffer -bench BenchmarkPooledAllocator -count 5 -benchtime 500ms
go run ./cmd/gnalloy-benchdiff -base HEAD~1 -packages ./channel -bench BenchmarkPipelineWriteAndFlushDirectSink$ -count 5 -benchtime 500ms -out benchdiff.md
go run ./cmd/gnalloy-benchdiff -base HEAD~1 -suite hotpath -count 5 -benchtime 500ms -out hotpath-benchdiff.md
```

`-base HEAD` 用于未提交改动和当前版本对比，`-base HEAD~1` 用于提交后复核与上一提交的
差异。报告使用每个 benchmark 的中位数计算 `ns/op`、`B/op` 和 `allocs/op` 变化率；
任何性能改动提交前必须保留同机、同 Go 版本、同命令的 before/after 结果。负数表示
候选版本在对应指标上下降，通常代表更快或分配更少。

`benchmarks/microbench` 维护稳定的 microbenchmark suite 目录。`hotpath` 覆盖
ByteBuf/allocator、Pipeline/Unsafe、codec、timer、queue、QUIC runtime 和观测热路径；
`native-io` 覆盖 TCP/UDP/raw 以及平台 completion helper，其中 io_uring 和 IOCP 场景只应
在对应原生平台运行。可用套件通过 `go run ./cmd/gnalloy-benchdiff -list-suites` 查看。

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

`benchmarks/parity/baseline.json` 是日常低成本对标入口：默认使用
`gnalloy-bench` 执行同模型 TCP echo 场景，同时保留 gnalloy 本地 microbench 场景；
Netty、gnet、netpoll 作为外部 harness 场景启用。Netty baseline 使用 epoll native
transport，避免拿 NIO 结果冒充 Linux native 对标。

正式 TCP echo 对标矩阵:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/tcp-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/tcp-matrix.json -out tcp-matrix-report.md
```

`benchmarks/parity/tcp-matrix.json` 覆盖 64B、1KiB、16KiB payload，包含 gnalloy
epoll、gnalloy io_uring 默认配置、gnalloy io_uring+mmap+fixed-buffer 优化配置、
Netty epoll、gnet、netpoll，并为每个正式场景设置一次 warmup 和三次 repeat。报告会
保留每次采样，同时汇总 throughput 的 min/median/max/mean、median ns/op 和总错误数。
runner 同时兼容 Go duration 和 Java `Duration` 文本，例如 Netty 输出中的
`PT2M22.38974812S`。

Windows TCP echo 对标矩阵:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/windows-tcp.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/windows-tcp.json -out windows-tcp-report.md
```

`benchmarks/parity/windows-tcp.json` 覆盖 gnalloy IOCP、Netty NIO 和 gnet。
CloudWeGo netpoll v0.7.5 在 Windows 上游实现为空，不能用于 Windows 性能结论；
Linux/macOS/BSD 才把 netpoll 纳入真实对标。

UDP echo 对标矩阵:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/linux-udp-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/linux-udp-matrix.json -out udp-matrix-report.md
```

Windows:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/udp-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/udp-matrix.json -out udp-matrix-report.md
```

Windows UDP 必须同时记录 `iocp` 和 `std`：当前本机样本显示 UDP 在 `std`
readiness 后端更快，而 TCP 仍以 IOCP completion 作为主要 Windows 后端。Linux UDP
必须记录 epoll 基线和 `SO_REUSEPORT` 多 socket 场景；Netty 使用 DatagramChannel，
gnet 使用 `udp://` event loop。CloudWeGo netpoll 当前没有等价 UDP server API，
UDP 表格中记录为不适用，不把失败命令伪造成性能结果。

2026-08-29 UDP echo 实测样本:

| 平台 | 场景 | median ops/s | median p99 ns | 结论 |
| --- | --- | ---: | ---: | --- |
| Windows/amd64 | gnalloy iocp 128B | 132,069 | 1,118,400 | 低于 Netty NIO，保留为 IOCP UDP 优化缺口。 |
| Windows/amd64 | gnalloy iocp 1KiB | 131,765 | 1,055,600 | 低于 Netty NIO，保留为 IOCP UDP 优化缺口。 |
| Windows/amd64 | gnalloy std 128B | 164,540 | 1,025,000 | 吞吐超过 Netty NIO 148,170 和 gnet 130,934。 |
| Windows/amd64 | gnalloy std 1KiB | 161,891 | 1,010,600 | 吞吐超过 Netty NIO 146,457 和 gnet 125,754。 |
| Linux/amd64 | gnalloy epoll 128B | 110,217 | 990,221 | 吞吐超过 Netty epoll 92,453，低于 gnet 175,317。 |
| Linux/amd64 | gnalloy epoll 1KiB | 105,983 | 1,007,715 | 吞吐超过 Netty epoll 90,485，低于 gnet 173,096。 |
| Linux/amd64 | gnalloy epoll reuseport 128B | 265,752 | 1,039,784 | 吞吐超过 Netty epoll 92,453 和 gnet 175,317。 |
| Linux/amd64 | gnalloy epoll reuseport 1KiB | 242,190 | 1,005,158 | 吞吐超过 Netty epoll 90,485 和 gnet 173,096。 |

这些数据只对同机、同 payload、64 个 connected UDP 客户端、每客户端 5000 条消息、
延迟采样率 1/64 的 echo 场景有效。Windows 报告路径为
`%TEMP%\gnalloy-udp-local-20260829-233828.json`；Linux 报告路径为
`/tmp/gnalloy-linux-udp-20260829-234423.json`。

HTTP/1.1 keep-alive 对标矩阵:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/linux-http1-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/linux-http1-matrix.json -out http1-matrix-report.md
```

Windows:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/windows-http1-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/windows-http1-matrix.json -out http1-matrix-report.md
```

Linux HTTP/1 矩阵固定 Gnalloy epoll、Netty epoll native transport、gnet poller 和
CloudWeGo netpoll poller。Windows 矩阵固定 Gnalloy IOCP、Netty NIO 和 gnet；
CloudWeGo netpoll v0.7.5 的 Windows server 路径返回 unsupported，不能用于本机
Windows 性能结论。

HTTP/2 stream 对标矩阵:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/linux-http2-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/linux-http2-matrix.json -out http2-matrix-report.md
```

Windows:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/http2-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/http2-matrix.json -out http2-matrix-report.md
```

HTTP/2 矩阵覆盖 h2c 明文和 TLS 1.3 + ALPN `h2` 密文，负载模型为 64 条连接、
每连接 5000 个顺序 stream、每场景 1 次 warmup 和 3 次正式采样。Gnalloy 使用
`codec/http2`、`handler/tls` 和平台 native TCP 后端；Netty 使用
`Http2FrameCodec`、`SslHandler` 和对应平台 transport。gnet 与 CloudWeGo netpoll
当前没有等价完整 HTTP/2 协议栈，HTTP/2 表格中记录为不适用，不把 TCP echo 结果
混入 HTTP/2 结论。

2026-08-30 HTTP/2 stream 实测样本:

| 平台 | 场景 | median ops/s | median p99 ns | 结论 |
| --- | --- | ---: | ---: | --- |
| Windows/amd64 | gnalloy IOCP h2c 128B | 242,992 | 1,033,400 | 吞吐超过 Netty NIO h2c 173,117。 |
| Windows/amd64 | gnalloy IOCP h2c 1KiB | 239,331 | 1,013,400 | 吞吐超过 Netty NIO h2c 180,774。 |
| Windows/amd64 | gnalloy IOCP h2 TLS 128B | 174,807 | 1,177,100 | 吞吐超过 Netty NIO h2 TLS 139,255。 |
| Windows/amd64 | gnalloy IOCP h2 TLS 1KiB | 171,053 | 1,221,100 | 吞吐超过 Netty NIO h2 TLS 134,112。 |
| Linux/amd64 | gnalloy epoll h2c 128B | 98,947 | 2,677,422 | 吞吐超过 Netty epoll h2c 72,836。 |
| Linux/amd64 | gnalloy epoll h2c 1KiB | 98,660 | 2,790,365 | 吞吐超过 Netty epoll h2c 75,421。 |
| Linux/amd64 | gnalloy epoll h2 TLS 128B | 65,950 | 2,575,618 | 吞吐超过 Netty epoll h2 TLS 45,916。 |
| Linux/amd64 | gnalloy epoll h2 TLS 1KiB | 66,258 | 2,262,359 | 吞吐超过 Netty epoll h2 TLS 42,255。 |

这些数据只对同机、同 payload、64 条 TCP 连接、每连接 5000 个顺序 HTTP/2 stream、
延迟采样率 1/64 的 request/response 场景有效。Windows 报告路径为
`%TEMP%\gnalloy-http2-local-20260830-010041.json`；Linux 报告路径为
`/tmp/gnalloy-http2-linux-20260830-0109.json`。

HTTP/3 stream 对标矩阵:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/linux-http3-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/linux-http3-matrix.json -out http3-matrix-report.md
```

Windows:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/http3-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/http3-matrix.json -out http3-matrix-report.md
```

HTTP/3 矩阵按 RFC 9114/RFC 9000 边界固定 TLS 1.3 和 ALPN `h3`，负载模型为
64 条 QUIC 连接、每连接 5000 个顺序 request stream、每场景 1 次 warmup 和 3 次
正式采样。Gnalloy 使用 `transport/quic`、`transport/http3` 和
`codec/http3`；Netty 使用 `Http3.newQuicServerCodecBuilder`、
`Http3.newQuicClientCodecBuilder` 和对应平台 DatagramChannel。gnet 与
CloudWeGo netpoll 当前没有等价完整 HTTP/3 协议栈，HTTP/3 表格中记录为不适用，
不把 TCP/UDP echo 结果混入 HTTP/3 结论。

2026-08-30 HTTP/3 stream 实测样本:

| 平台 | 场景 | median ops/s | median ns/op | median p99 ns | errors | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| Windows/amd64 | gnalloy rfc9000 h3 128B | 46,660 | 21,432 | 1,783,540 | 0 | 吞吐超过 Netty NIO h3 27,922。 |
| Windows/amd64 | gnalloy rfc9000 h3 1KiB | 46,179 | 21,655 | 1,798,882 | 0 | 吞吐超过 Netty NIO h3 27,245。 |
| Linux/amd64 | gnalloy rfc9000 h3 128B | 33,248 | 30,077 | 3,182,043 | 0 | 吞吐超过 Netty epoll h3 11,812。 |
| Linux/amd64 | gnalloy rfc9000 h3 1KiB | 32,005 | 31,245 | 3,402,783 | 0 | 吞吐超过 Netty epoll h3 11,727。 |
| Windows/amd64 | Netty NIO h3 128B | 27,922 | 35,813 | 3,324,000 | 0 | Windows NIO 对照样本。 |
| Windows/amd64 | Netty NIO h3 1KiB | 27,245 | 36,704 | 3,912,300 | 0 | Windows NIO 对照样本。 |
| Linux/amd64 | Netty epoll h3 128B | 11,812 | 84,663 | 35,438,382 | 0 | Linux epoll 对照样本。 |
| Linux/amd64 | Netty epoll h3 1KiB | 11,727 | 85,270 | 35,598,725 | 0 | Linux epoll 对照样本。 |

这些数据只对同机、同 payload、64 条 QUIC 连接、每连接 5000 个顺序 HTTP/3 request
stream、延迟采样率 1/64 的 request/response 场景有效。Windows 报告路径为
`%TEMP%\gnalloy-http3-local-20260830-092504.json`；Linux 报告路径为
`/tmp/gnalloy-http3-linux-20260830-093214.json`。

TLS 版本矩阵:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/linux-tls-version-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/linux-tls-version-matrix.json -out tls-version-matrix-report.md
```

Windows:

```powershell
.\scripts\build-external-bench.ps1
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/tls-version-matrix.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/tls-version-matrix.json -out tls-version-matrix-report.md
```

TLS 矩阵覆盖 HTTPS/1.1 的 TLS 1.1、1.2、1.3，以及 HTTP/2 over TLS 的
TLS 1.2、1.3。HTTP/2 不纳入 TLS 1.1：这不是实现取舍，而是协议安全边界。
Gnalloy harness 使用 `-tls-version` 精确固定 `crypto/tls` 的
`MinVersion/MaxVersion`，Netty harness 使用 `--tls-version` 固定
`SslHandler` 的协议版本，并在 Java 21 进程内仅对显式 TLS 1.1 场景放开
`TLSv1.1` 禁用项，避免影响默认 TLS 1.3 路径。TLS 1.1/1.2 场景同时
通过 `-cipher-suites`/`--cipher-suites` 固定双方共同支持的 IANA/Java
密码套件；TLS 1.3 和 HTTP/3 场景保留运行时/provider 默认密码套件，因为
Go `crypto/tls` 不允许把 TLS 1.3 套件写入 `CipherSuites`。HTTPS/1 的
Gnalloy 场景使用 `-http1-mode raw`，仍经过 `handler/tls`，只去掉通用
HTTP/1 对象编解码成本；HTTPS/2 场景保持 `codec/http2`。
gnet-bench 当前只接受 `tcp-echo`、`udp-echo` 和 `http1`；netpoll-bench 当前只接受
`tcp-echo` 和 `http1`。两者没有原生 HTTP/2、HTTP/3 或 TLS/ALPN 协议栈场景，
因此 TLS 1.1/1.2/1.3 表格只对 Gnalloy 和 Netty 做同协议对比。

2026-08-30 HTTP/1 和 HTTPS/1 串行优化后实测样本:

| 平台 | 场景 | median ops/s | median ns/op | median p99 ns | 错误数 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| Windows/amd64 | gnalloy IOCP HTTP/1 raw 128B | 405,793 | 2,464 | 1,017,400 | 0 | 高于 Netty NIO 322,557。 |
| Windows/amd64 | gnalloy IOCP HTTP/1 raw 1KiB | 403,909 | 2,476 | 1,018,000 | 0 | 高于 Netty NIO 309,131 和 gnet 377,681；Windows netpoll 上游不支持。 |
| Linux/amd64 | gnalloy epoll HTTP/1 raw 128B | 235,580 | 4,245 | 2,955,722 | 0 | 高于 Netty epoll 143,566。 |
| Linux/amd64 | gnalloy epoll HTTP/1 raw 1KiB | 233,266 | 4,287 | 3,149,984 | 0 | 高于 Netty epoll 137,876、gnet 175,118 和 netpoll 227,571。 |
| Windows/amd64 | gnalloy IOCP HTTPS/1 raw TLS 1.1 128B | 245,955 | 4,066 | 1,011,000 | 0 | 高于 Netty NIO 199,547。 |
| Windows/amd64 | gnalloy IOCP HTTPS/1 raw TLS 1.1 1KiB | 231,620 | 4,317 | 1,008,900 | 0 | 高于 Netty NIO 186,623。 |
| Linux/amd64 | gnalloy epoll HTTPS/1 raw TLS 1.1 128B | 89,676 | 11,151 | 2,220,167 | 0 | 高于 Netty epoll 70,384。 |
| Linux/amd64 | gnalloy epoll HTTPS/1 raw TLS 1.1 1KiB | 81,222 | 12,312 | 2,483,860 | 0 | 高于 Netty epoll 60,836。 |
| Windows/amd64 | gnalloy IOCP HTTPS/1 raw TLS 1.2 128B | 277,140 | 3,608 | 1,011,700 | 0 | 高于 Netty NIO 205,560。 |
| Windows/amd64 | gnalloy IOCP HTTPS/1 raw TLS 1.2 1KiB | 267,858 | 3,733 | 1,016,100 | 0 | 高于 Netty NIO 181,015。 |
| Linux/amd64 | gnalloy epoll HTTPS/1 raw TLS 1.2 128B | 127,201 | 7,862 | 1,389,569 | 0 | 高于 Netty epoll 63,401。 |
| Linux/amd64 | gnalloy epoll HTTPS/1 raw TLS 1.2 1KiB | 124,532 | 8,030 | 1,401,373 | 0 | 高于 Netty epoll 64,232。 |
| Windows/amd64 | gnalloy IOCP HTTPS/1 raw TLS 1.3 128B | 277,985 | 3,597 | 1,011,100 | 0 | 高于 Netty NIO 196,292。 |
| Windows/amd64 | gnalloy IOCP HTTPS/1 raw TLS 1.3 1KiB | 270,447 | 3,698 | 1,015,000 | 0 | 高于 Netty NIO 184,625。 |
| Linux/amd64 | gnalloy epoll HTTPS/1 raw TLS 1.3 128B | 127,642 | 7,834 | 1,399,871 | 0 | 高于 Netty epoll 61,599。 |
| Linux/amd64 | gnalloy epoll HTTPS/1 raw TLS 1.3 1KiB | 124,966 | 8,002 | 1,458,223 | 0 | 高于 Netty epoll 57,888。 |

这些数据按协议串行执行，未并发跑多个矩阵；每个场景 3 次样本取 median。
HTTP/1 raw 矩阵使用连接数 128、每连接 10000 个顺序 request、warmup 1000、
延迟采样率 1/128，并为固定 GET 请求头设置 384B read buffer。HTTPS/1 raw TLS
矩阵使用连接数 64、每连接 5000 个顺序 request、warmup 500、延迟采样率 1/64，
同样为固定 GET 请求头设置 384B read buffer。
HTTP/1 Windows 报告路径为
`%TEMP%\gnalloy-windows-http1-gnet-20260830-continue.md`，Linux 报告路径为
`/tmp/gnalloy-linux-http1-epoll-gnet-netpoll-20260830-continue.md`。TLS 1.1/1.2/1.3
Windows 报告路径为 `%TEMP%\gnalloy-tls-raw-local-20260830-144416.md`，
Linux 报告路径为 `/tmp/gnalloy-tls-raw-linux-20260830-145419.md`。
TLS 矩阵使用外部 harness 原生命令按版本分段串行执行，未把一次性机器负载样本外推为通用性能结论。

harness 源码位于 `benchmarks/external`，构建产物输出到 `benchmarks/external/bin`；
该目录是本机构建产物，不提交到仓库。Netty 使用独立 Maven 工程，
gnalloy/gnet/netpoll 使用独立 Go module，避免把对标依赖引入 gnalloy 根 `go.mod`。

严格外部对标 gate:

```bash
./scripts/build-external-bench.sh
go run ./examples/parity-bench -dry-run -strict-external -config benchmarks/parity/baseline.json
```

`-strict-external` 用于正式声称同机对标前的合同检查：`parity-harness` tag
标记的 gnalloy 场景、Netty、gnet、netpoll 以及带 `external` tag 的场景不能保持
`skip=true`，展开变量后的命令入口必须在本机可解析；`java -jar` 场景还会检查
jar 文件本身是否存在。Windows 下 Go harness 构建为 `.exe`，baseline 仍使用无扩展名
路径，由 strict gate 和 runner 按平台解析。

外部 harness 支持的最小命令合同:

```bash
benchmarks/external/bin/gnalloy-bench -protocol tcp-echo -payload 1024 -connections 256 -messages 100000 -backend default
benchmarks/external/bin/gnalloy-bench -protocol http3 -payload 1024 -connections 64 -messages 5000 -alpn h3 -tls-version 1.3
java -jar benchmarks/external/bin/netty-bench.jar --protocol tcp-echo --payload 1024 --connections 256 --messages 100000
java -jar benchmarks/external/bin/netty-bench.jar --protocol http3 --payload 1024 --connections 64 --messages 5000 --alpn h3 --tls-version 1.3
benchmarks/external/bin/gnet-bench -protocol tcp-echo -payload 1024 -connections 256 -messages 100000
benchmarks/external/bin/netpoll-bench -protocol tcp-echo -payload 1024 -connections 256 -messages 100000
```

`gnalloy-bench` 的 `-workers 0` 表示按后端自动选择 worker 数；Linux epoll/io_uring
会把自动值限制在 4，Windows IOCP 会把自动值限制在 8，避免过多 EventLoop 在
native poller/完成端口上竞争导致吞吐退化。
`-read-buffer-size 0` 表示按 `max(payload, 4096)` 自动设置单次读缓冲区，避免
大包 TCP echo 被默认 4KiB 缓冲拆成多次 read completion。
Linux 高性能场景可以显式打开 `-mmap`、`-iouring-fixed-buffers`、
`-iouring-multishot-accept` 和 `-reuseport`。fixed-buffer 场景必须同时使用
`-backend iouring -mmap`，且 read buffer 不能超过 mmap block size，避免矩阵把
不成立的组合误报成性能结果。
`-latency-sample-rate N` 用于按连接采样端到端 RTT，输出 `p50LatencyNs`、
`p95LatencyNs`、`p99LatencyNs`、`p999LatencyNs` 和 `maxLatencyNs`；默认 0
表示关闭逐请求 `time.Now()`，避免污染纯吞吐基线。harness 还会输出当前进程的
`rssBytes`、`heapAllocBytes`、`heapSysBytes`、`heapObjects`、`gcCount`、
`gcPauseNs` 和 `goroutines`，用于判断吞吐提升是否靠资源透支换来。

harness 输出 `framework=... total=... errors=... throughput=... ops/s` 汇总行和
Go benchmark 兼容的 `Benchmark* ns/op` 行，分别用于报告吞吐/错误数和固定连接数、
固定消息数下的端到端 TCP echo RTT 均值。外部进程无法使用 Go `benchmem` 的内存口径
准确统计框架内部分配，因此不伪造 `B/op` 和 `allocs/op`。

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
- `transport/quic` 通过 quic-go 提供 RFC9000/TLS1.3 互通连接栈，不保留自研 ACK/loss/congestion/runtime pipeline，也不暴露独立 provider 子包。
- `transport/quic/application` 提供 QUIC stream/datagram 应用协议装配；`resolver/dns/quic` 以 DNS-over-QUIC 覆盖真实应用协议用例。
- `transport/l2` 已提供可注入 Driver 的二层帧 transport 抽象；Linux AF_PACKET、macOS/BSD BPF、Windows Npcap 均在核心库提供 native driver，运行时验证需要本机权限、接口环境变量和对应平台驱动。
- `scripts/platform-matrix.json` 是跨平台验证事实源；`scripts/verify-platform.ps1 -SkipBench -ReportPath platform-report.json` 会输出 passed/skipped/failed gate 结果。
- `observability.PrometheusExporter` 是无依赖文本导出器；OpenTelemetry 已作为 `observability/otel` adapter 接入。
