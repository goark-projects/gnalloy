# gnalloy

`gnalloy` is a Go-native, Netty-inspired network framework foundation.

Install:

```bash
go get goark.dev/gnalloy
```

Local development:

- The canonical module path is `goark.dev/gnalloy`; repository hosting can stay
  on GitHub behind the domain mapping.
- Run repository checks from this checkout with `GOWORK=off` when an outer
  workspace could affect module resolution.
- If another local module needs this unpublished checkout during development,
  add a temporary replace rule in that module:

```go
replace goark.dev/gnalloy => G:/opensource/goark/gnalloy
```

The current implementation provides stable contracts and hot-path building
blocks:

- `buffer`: reference-counted `ByteBuf`, zero-copy slices, readable slice views,
  composite buffers, heap allocator, cross-platform size-class pooled
  allocator, stat allocator, and Linux mmap slab allocator.
- `codec`: Netty-style codec foundations including `ByteToMessageDecoder`,
  composite/merge cumulators, `MessageToByteEncoder`, message-to-message
  codecs, combined duplex handler, length-field, delimiter, line,
  fixed-length, matching outbound frame encoders, string, and byte-slice
  codecs.
- `codec/compression`: gzip and zlib ByteBuf encoders/decoders backed by the
  Go standard library, with explicit decoded-size limits.
- `codec/dns`, `codec/http1`, `codec/http2`, `codec/http3`,
  `codec/protobuf`, `codec/mqtt`, `codec/redis`, and `codec/websocket`:
  protocol codec coverage for DNS, HTTP/1.x, HTTP/2 binary frames, HPACK
  header blocks, HTTP/2 stream child channel flow, HTTP/3 frames, QPACK header
  blocks, HTTP/3 control/QPACK stream pipelines, WebTransport SETTINGS and
  extended CONNECT helpers, Protobuf varint32 frames, MQTT frames, Redis RESP
  frames, and WebSocket frames.
- `channel`: inbound/outbound pipeline contracts, `Group`/`GroupHandler`, and
  `FileRegion` fallback encoding; the `Unsafe` bridge normalizes
  readiness/completion events before they enter business handlers.
- `channel/pool`: `SimplePool`, `FixedPool`, and `Map` Channel reuse
  primitives with explicit factory, health check, lifecycle callback, return,
  discard, timeout, and close semantics.
- `handler/timeout`: time-wheel based `IdleStateHandler`,
  `ReadTimeoutHandler`, and `WriteTimeoutHandler` without per-connection
  `time.Timer` allocation.
- `handler/tls`: Go-native TLS handler backed by `crypto/tls`, exposing
  plaintext `ByteBuf` to business handlers while preserving SNI and ALPN
  negotiation events; StartTLS, SNI-driven config selection, stable ByteBuf
  copy reduction, and optional native TLS capability evaluation are handled as
  explicit pipeline controls.
- `handler/ipfilter`: ordered allow/deny rules for CIDR, single IP, UDP/raw
  messages, and custom remote-address providers.
- `handler/pcap`: pipeline-level libpcap capture for inbound and outbound
  payloads without taking message ownership.
- `handler/proxy`: HTTP CONNECT client handler plus SOCKS5 and HAProxy v1/v2
  wire helpers for proxy negotiation and source-address metadata.
- `handler/metrics` and `observability`: vendor-neutral Channel metrics
  contracts, an atomic low-overhead recorder, a Pipeline handler for lifecycle,
  read/write byte, flush, close, and exception counters, plus Prometheus text
  export and an OpenTelemetry adapter.
- `queue`: bounded CAS-based MPSC ring queue for cross-EventLoop delivery.
- `resolver/dns`: Go-native DNS resolver with system fallback, explicit
  exchanger hooks, UDP query support, TCP fallback, hosts override,
  search-domain/ndots expansion, bounded CNAME follow, cache clearing, and
  A/AAAA lookup helpers.
- `timer`: local hashed wheel timer for idle and heartbeat checks.
- `transport`: EventLoop, Channel identity contracts, and a thin factory over
  split poller packages.
- `bootstrap`: Netty-style `ServerBootstrap`, Boss/Worker `EventLoopGroup`,
  `ChildHandler`, and pluggable server transport binding.
- `transport/tcp`: native TCP lifecycle backed by platform socket APIs:
  Linux `socket/bind/listen/accept4`, macOS/BSD `socket/bind/listen/accept`,
  and Windows `WSASocket/bind/listen` with IOCP `AcceptEx` request support.
  Linux/macOS/BSD can enable `SO_REUSEPORT` to create one listen socket per
  Boss `EventLoop`.
- `transport/poller`: backend-neutral Poller API, including a cross-platform
  `std` readiness fallback.
- `transport/poller/epoll`: Linux epoll ET readiness backend.
- `transport/poller/iouring`: Linux io_uring completion backend with
  accept/read/write/close SQE support, configurable ring entries and optional
  SQPOLL setup.
- `transport/poller/kqueue`: macOS/BSD kqueue readiness backend.
- `transport/poller/iocp`: Windows IOCP completion backend with
  AcceptEx/WSARecv/WSASend close completion support.
- `transport/quic`: QUIC packet/runtime engine over UDP, including packet
  encode/decode, connection-ID routing, frame dispatch, ACK range tracking,
  packet-threshold loss detection, Reno-style congestion window, stream
  flow-control state, and path validation/migration state.
- `transport/quic/rfc9000`: RFC 9000 QUIC v1 connection adapter backed by a
  mature TLS 1.3 packet-protection stack, exposing bidirectional streams,
  unidirectional streams for HTTP/3 control/QPACK integration, datagrams,
  connection state, localhost interop tests, and opt-in external interop tests.
- `transport/http3`: HTTP/3 transport binding that maps RFC9000 QUIC request,
  control, and QPACK streams into gnalloy `Channel` pipelines.
- `transport/webtransport`: WebTransport over HTTP/3 session binding with
  CONNECT stream session IDs, bidirectional/unidirectional stream prefixes,
  QUIC datagram mapping, and negotiated capability validation.

Examples:

```bash
go run ./examples/echo -addr :9000 -backend default -workers 4
go run ./examples/length-field -addr :9001 -backend default -workers 4
go run ./examples/line-frame -addr :9003 -backend default -workers 4
go run ./examples/fixed-length -addr :9004 -backend default -workers 4 -frame-length 4
```

Netty parity:

- Netty 对标总览见 `docs/netty-parity.md`。
- Netty codec 对齐清单见 `docs/netty-codec-parity.md`。
- Transport completion 支持矩阵见 `docs/transport-completion-matrix.md`。
- Benchmark parity 口径见 `docs/benchmark-parity.md`。

Verification:

```bash
go test ./...
./scripts/verify-regression.sh
./scripts/verify-bench.sh
go run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json
GROUPS=codec,queue,timer ./scripts/verify-bench.sh
```

PowerShell:

```powershell
go test ./...
.\scripts\verify-regression.ps1
.\scripts\verify-bench.ps1
.\scripts\verify-platform.ps1 -SkipBench -ReportPath platform-report.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json
.\scripts\verify-bench.ps1 -Groups codec,queue,timer
```

Example flags shared by the TCP examples:

- `-addr`: listen address.
- `-backend`: `default`, `std`, `epoll`, `iouring`, `kqueue`, `iocp`, or `memory`.
- `-boss`: Boss EventLoop count.
- `-workers`: Worker EventLoop count.
- `-reuseport`: enable `SO_REUSEPORT` on supported Unix platforms.
- `-mmap`: use the per-worker mmap allocator when supported.
- `-mmap-block-size` / `-mmap-blocks`: mmap slab layout.
- `-iouring-entries`: io_uring queue depth.
- `-iouring-sqpoll`, `-iouring-sqpoll-affinity`,
  `-iouring-sqpoll-cpu`, `-iouring-sqpoll-idle-ms`: io_uring SQPOLL options.
- `-iouring-multishot-accept`: enable one accept SQE to produce multiple CQEs
  when the running Linux kernel supports it.
- `-iouring-fixed-buffers`: register per-worker mmap allocator blocks as
  io_uring fixed buffers. This is intentionally supported only with
  `-backend iouring -mmap`; other combinations fail fast.

Smoke verification:

```bash
go run ./examples/smoke-client -addr 127.0.0.1:9000 -protocol raw -count 3
go run ./examples/smoke-client -addr 127.0.0.1:9001 -protocol length-field -count 3
go run ./examples/smoke-client -addr 127.0.0.1:9003 -protocol line -count 3
go run ./examples/smoke-client -addr 127.0.0.1:9004 -protocol fixed -count 3
```

Benchmark client:

```bash
go run ./examples/bench-client -addr 127.0.0.1:9000 -protocol raw -connections 256 -messages 1000 -payload-size 64
go run ./examples/bench-client -addr 127.0.0.1:9001 -protocol length-field -connections 256 -messages 1000 -payload-size 64
go run ./examples/bench-client -addr 127.0.0.1:9003 -protocol line -connections 256 -messages 1000 -payload-size 64
go run ./examples/bench-client -addr 127.0.0.1:9004 -protocol fixed -connections 256 -messages 1000 -payload-size 4
```

Stress client:

```bash
go run ./examples/stress-client -addr 127.0.0.1:9000 -protocol raw -scenario mixed -connections 128 -messages 128 -payload-size 64
go run ./examples/stress-client -addr 127.0.0.1:9001 -protocol length-field -scenario mixed -connections 128 -messages 128 -payload-size 64
go run ./examples/stress-check -backend default -protocol both -scenario mixed -connections 128 -messages 128 -payload-size 64
```

Stress scenarios:

- `long`: long-lived connections with repeated echo exchanges.
- `short`: repeated connect/exchange/close cycles.
- `half-frame`: split writes to expose TCP half-packet handling.
- `slow`: byte-by-byte writes to simulate slow clients.
- `mixed`: runs all scenarios above.

Platform helper scripts:

```powershell
.\scripts\verify-platform.ps1
.\scripts\verify-platform.ps1 -SkipBench -ReportPath platform-report.json
.\scripts\verify-smoke.ps1 -Backend default -Workers 2
.\scripts\verify-stress.ps1 -Backend iocp -Workers 2
.\scripts\verify-iocp.ps1
```

```bash
./scripts/verify-smoke.sh
./scripts/verify-stress.sh
./scripts/verify-iouring-sqpoll.sh
./scripts/verify-iouring-fixed.sh
BACKEND=epoll WORKERS=4 REUSEPORT=1 ./scripts/verify-smoke.sh
BACKEND=iouring WORKERS=4 MMAP=1 ./scripts/verify-smoke.sh
BACKEND=kqueue WORKERS=4 ./scripts/verify-smoke.sh
BACKEND=epoll WORKERS=4 REUSEPORT=1 ./scripts/verify-stress.sh
BACKEND=iouring WORKERS=4 MMAP=1 IOURING_MULTISHOT_ACCEPT=1 ./scripts/verify-stress.sh
BACKEND=iouring WORKERS=2 MMAP=1 MMAP_BLOCKS=512 IOURING_FIXED_BUFFERS=1 ./scripts/verify-stress.sh
BACKEND=kqueue WORKERS=4 ./scripts/verify-stress.sh
```

Benchmarks:

```bash
go test -bench=. ./buffer ./channel ./codec ./queue ./timer ./transport/tcp
./scripts/verify-bench.sh
BACKENDS=epoll,iouring MMAP=1 MMAP_BLOCKS=512 IOURING_MULTISHOT_ACCEPT=1 IOURING_FIXED_BUFFERS=1 ./scripts/verify-bench.sh
```

```powershell
.\scripts\verify-bench.ps1
.\scripts\verify-bench.ps1 -Backends default,iocp
```

Current hot-path target:

- `BenchmarkLengthFieldDecoder` is expected to stay at `0 B/op` and
  `0 allocs/op` when input ByteBuf is supplied by an allocator.

Backend matrix:

| Backend | Platform | Model | Status |
| --- | --- | --- | --- |
| `epoll` | Linux | readiness / Reactor | implemented, ET mode |
| `iouring` | Linux | completion / Proactor | implemented for accept/read/write/close |
| `kqueue` | macOS/BSD | readiness / Reactor | implemented |
| `iocp` | Windows | completion / Proactor | implemented for AcceptEx/WSARecv/WSASend/close |
| `std` | all | readiness / polling fallback | implemented |
| `memory` | all | in-process test poller | implemented for tests |

Transport-level readiness/completion coverage is tracked in
`docs/transport-completion-matrix.md`.

Current validation boundary:

- Windows native tests, race tests, vet, cross compilation, and smoke scripts are
  supported from this repository. `scripts/platform-matrix.json` is the
  machine-readable cross-platform gate source, and `validation/platformmatrix`
  keeps the matrix structurally tested.
- Linux and macOS runtime behavior must be validated on the corresponding
  machines with `scripts/verify-smoke.sh`, because cross compilation only proves
  compile-time compatibility.
- `mmap` allocator refuses to close while buffers are still in use. This is
  intentional: unmapping memory that an active `ByteBuf` still references is
  unsafe.
- `buffer.StatAllocator` exposes allocator stats for leak checks. TCP servers
  expose cached worker allocator stats through `AllocatorStats()` when callers
  need runtime observability.
- `handler/metrics.ChannelMetricsHandler` records Channel lifecycle, read/write
  bytes, flush, close, and exception counters through
  `observability.ChannelRecorder`; the bundled atomic recorder is a low-cardinality
  default for smoke tests and embedded exporters.
- `examples/stress-check` starts the server in-process and fails when active
  connections or allocator in-use counts do not drain to zero.
- `Server.Close` stops acceptors, closes tracked active child channels, waits for
  inactive hooks, and only then closes cached per-worker allocators.
- io_uring fixed buffers are registered and unregistered on the owning
  `EventLoop`; mmap allocators are not unmapped until fixed buffers have been
  unregistered and all active child channels have drained.
- Registered buffers may be constrained by Linux locked-memory limits; reduce
  `MMAP_BLOCKS` or raise the process memlock limit when fixed-buffer setup
  returns a kernel permission/resource error.

Design rules:

- A `Channel` is owned by exactly one `EventLoop`.
- Public handlers never see file descriptors or transport-specific events.
- Readiness transports such as epoll and completion transports such as
  io_uring are normalized at `channel.Unsafe`, not in the business pipeline.
- `ByteBuf` slices retain their parent memory and never copy payload bytes.
- Cross-thread operations are submitted to the owner loop.
- Native system calls go through `golang.org/x/sys/unix` or
  `golang.org/x/sys/windows`; the platform gate only allows the documented
  legacy stdlib `syscall` shim in Windows TCP socket setup.
- Outbound writes are queued per Channel; partial writes remain in the outbound
  buffer until a write-ready/completion event drains them.
- Outbound buffers are gathered across queued `ByteBuf` instances. Readiness
  transports use `writev`/multi-buffer `WSASend`, while completion transports
  can submit batched buffers through io_uring `WRITEV` or IOCP `WSASend`.
- io_uring supports optional registered buffers and multishot accept at the
  poller layer; both are opt-in because kernel/version support differs.
- Performance-only CPU affinity is configured on `EventLoopGroup`; Linux binds
  via `runtime.LockOSThread` and `x/sys/unix.SchedSetaffinity`, while unsupported
  platforms fail only when affinity is explicitly requested.
- TCP servers track active child channels so shutdown can drain connections
  before closing off-heap allocators.
