# gnalloy Production Runbook

本文档定义 gnalloy 在生产环境启用 TCP、UDP、raw IP、L2 frame、QUIC、HTTP/3
和 WebTransport 前必须完成的运行检查。这里记录的是运行边界，不替代业务协议
自身的安全、鉴权、限流和审计设计。

## Release Gate

发布前至少执行以下门禁：

```sh
go test ./...
./scripts/verify-regression.sh
./scripts/verify-protocol.sh
ALLOW_SKIP=1 ./scripts/verify-privileged.sh
./scripts/verify-soak.sh
./scripts/verify-bench.sh
```

Windows:

```powershell
go test ./...
.\scripts\verify-regression.ps1
.\scripts\verify-protocol.ps1 -ReportPath protocol-report.json
.\scripts\verify-privileged.ps1 -AllowSkip -ReportPath privileged-report.json
.\scripts\verify-soak.ps1 -ReportPath soak-report.json
.\scripts\verify-bench.ps1
```

正式声明外部性能对标前，还必须在同一台机器上打开 Netty、gnet、netpoll
harness，并运行：

```sh
go run ./examples/parity-bench -strict-external -config benchmarks/parity/baseline.json
go run ./examples/parity-bench -strict-external -config benchmarks/parity/tcp-matrix.json -out tcp-matrix-report.md
```

`baseline.json` 适合作为日常合同和单点 smoke；`tcp-matrix.json` 是正式 TCP echo
对外性能声明的最低矩阵。对外报告必须附带 OS、CPU、Go/JVM 版本、GOMAXPROCS、
JVM GC、Netty `EventLoopGroup`、Netty allocator、native transport、payload、
连接数、消息数、warmup/repeat、错误率和延迟分位。

## Platform Matrix

| Platform | Required runtime checks |
| --- | --- |
| Linux | `epoll` TCP/UDP/raw smoke；可选 `io_uring`、SQPOLL、fixed buffers；raw/L2 需要 `CAP_NET_RAW` 或等效权限。 |
| macOS/BSD | `kqueue` TCP/UDP/raw smoke；L2 BPF 需要可打开 `/dev/bpf*` 且目标网卡允许捕获。 |
| Windows | IOCP TCP/UDP smoke；L2 Npcap 需要安装 Npcap，运行用户需要抓包权限，`wpcap.dll` 必须能被系统加载。 |

跨编译只证明构建边界，不证明运行边界。生产上线前必须在目标系统上执行对应
native smoke、stress、privileged gate。

## Transport Rules

TCP/UDP:

- TCP 和 UDP 默认走 `bootstrap.ServerBootstrap`、`bootstrap.Dialer`、Pipeline
  和 `ChannelOption`，不要绕过 `channel.Unsafe` 直接写 socket。
- 长连接业务必须配置读写超时、空闲检测或业务心跳，避免半开连接长期占用
  `EventLoop` 和 allocator。
- 批量写入优先通过 Pipeline 和 flush 合并，不在 handler 中自建无界 goroutine。

raw IP:

- Linux 生产环境优先给二进制授予最小 `CAP_NET_RAW`，不要直接以 root 运行完整
  业务进程，除非部署平台无法拆分权限。
- raw 协议号必须固定并纳入变更审批；业务自定义 IP 协议要记录协议号、报文
  版本、最大包长和回退策略。
- raw socket 行为受操作系统策略影响，Windows/macOS 可能限制部分协议族或发送
  语义，必须以目标系统实测结果为准。

L2 frame:

- Linux 使用 AF_PACKET；macOS/BSD 使用 BPF；Windows 使用 Npcap。三者都通过
  `transport/l2.Driver` 进入核心，BPF/Npcap native 细节保持在
  `transport/l2/internal/nativeframe`。
- L2 帧 payload 表示完整二层帧字节，业务层必须明确 EtherType、源/目标 MAC、
  VLAN 和 MTU 边界。
- 抓包/注入二层帧可能绕过主机协议栈安全控制，生产环境要限制网卡、命名空间、
  容器 capability 和运维账号权限。

QUIC/HTTP3/WebTransport:

- QUIC RFC9000 连接必须配置 TLS 1.3、ALPN、证书校验和目标 ServerName。
- DNS-over-QUIC 使用 RFC 9250 ALPN `doq` 和默认端口 `853`，外部验证通过
  `GNALLOY_DOQ_ADDR` 显式开启。
- HTTP/3 和 WebTransport 依赖 QUIC stream/datagram 能力，生产前必须验证
  本机 TLS、ALPN、datagram capability 和负载均衡器 UDP 转发策略。
- 0-RTT 只在业务幂等且已完成重放风险评估后开启。

## Performance And Stability

- `scripts/verify-soak.*` 默认只跑一个短周期。正式发布前按业务峰值模型设置
  `GNALLOY_SOAK_DURATION_SECONDS` 或 `-DurationSeconds`，并保存 report。
- 压测结果必须记录 OS、CPU、Go 版本、GOMAXPROCS、后端、worker 数、payload、
  连接数、场景、错误率和 P99/P999 延迟。
- 和 Netty 对比时必须额外记录 Java 版本、GC、heap 参数、`EventLoopGroup`、
  allocator、native transport 依赖版本和 handler pipeline；缺少这些字段时只能
  作为本地探索结果，不能作为发布声明。
- `io_uring` fixed buffers 必须和 mmap allocator 一起启用；memlock 限制不足时
  应调整系统限制或关闭 fixed buffers，不允许静默降级后仍声称 fixed-buffer
  结果。
- 回压验证必须覆盖慢消费者、半包、短连接 churn 和写队列水位线。

## Failure Handling

常见故障处理：

- `permission denied` 或 raw/L2 bind 失败：检查管理员权限、`CAP_NET_RAW`、BPF
  设备权限、Npcap 安装和目标接口名称。
- Npcap 加载失败：确认安装 Npcap 兼容模式，`wpcap.dll` 位数和进程一致，服务
  已启动。
- QUIC 握手失败：检查 TLS 证书、ALPN、ServerName、UDP 防火墙、NAT 和负载均衡
  超时。
- allocator close 失败：说明仍有 buffer 未释放或连接未 drain，先定位泄漏的
  handler，再扩大 `DrainTimeout` 只是临时止血。
- soak 中后段错误率上升：优先检查端口耗尽、TIME_WAIT、fd 上限、内存限制、
  GC 暂停和 EventLoop 队列积压。

## Rollback

- 新 transport backend 上线必须可切回 `default` backend。
- raw/L2/QUIC 新业务协议必须保留旧协议监听或灰度开关，至少覆盖一次完整业务
  周期。
- 回滚后重新执行 smoke 和协议级请求，确认旧链路恢复，而不是只看进程存活。
