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
  Go standard library, plus isolated brotli, snappy, lz4, zstd, bzip2, and
  lzma extension subpackages with explicit decoded-size limits.
- `codec/dns`, `codec/http1`, `codec/http1/cookie`,
  `codec/http1/multipart`, `codec/http2`, `codec/http3`, `codec/sctp`,
  `codec/protobuf`, `codec/mqtt`, `codec/redis`, `codec/websocket`, and
  `codec/websocket/deflate`:
  protocol codec coverage for DNS, HTTP/1.x, HTTP/1 query string helpers,
  HTTP/1 content compression/decompression, HTTP/1 Upgrade helpers, HTTP/2
  binary frames, client preface, SETTINGS ACK lifecycle, outbound DATA
  flow-control queuing, h2c Upgrade settings, HPACK header blocks, HTTP/1
  object bridge, HTTP/1 Cookie/Set-Cookie values, HTTP/2 stream child channel
  flow, bounded multipart/form-data decode/encode,
  HTTP/3 frames, lifecycle events, low-cardinality frame stats, QPACK header
  blocks, HTTP/3 control/QPACK stream pipelines,
  SCTP stream messages and byte-stream adapters, WebTransport SETTINGS and
  extended CONNECT helpers, Protobuf object adapters and varint32 frames, MQTT
  frames, Redis RESP frames, Memcached binary full request/response objects,
  WebSocket frames, and permessage-deflate extension compression.
- `channel`: inbound/outbound pipeline contracts, `Group`/`GroupHandler`,
  direct `FileRegion` outbound writes through a pluggable native writer, and
  fallback chunk encoding with optional native source metadata; the `Unsafe`
  bridge normalizes readiness/completion events before they enter business
  handlers.
- `channel/pool`: `SimplePool`, `FixedPool`, and `Map` Channel reuse
  primitives with explicit factory, health check, lifecycle callback, return,
  discard, timeout, and close semantics.
- `handler/timeout`: time-wheel based `IdleStateHandler`,
  `ReadTimeoutHandler`, and `WriteTimeoutHandler` without per-connection
  `time.Timer` allocation.
- `handler/tls`: Go-native TLS handler backed by `crypto/tls`, exposing
  plaintext `ByteBuf` to business handlers while preserving SNI and ALPN
  negotiation events; StartTLS, SNI-driven config selection, stable ByteBuf
  copy reduction, ClientHello inspection/provider-based config selection,
  Optional TLS detection, TLS 1.0-1.2 cipher-suite catalog/name
  parsing/configuration, stapled OCSP response policy/events, and optional
  native TLS capability evaluation are handled as explicit pipeline controls.
- `handler/ipfilter`: ordered allow/deny rules for CIDR, single IP, UDP/raw
  messages, and custom remote-address providers.
- `handler/cors`: HTTP/1 CORS Pipeline handler with request Origin matching,
  preflight short-circuit responses, credential-safe wildcard behavior, and
  response header decoration.
- `handler/flow`: Pipeline-level inbound flow-control handler with explicit
  pause/resume, bounded pending message/byte budgets, read-complete coalescing,
  AutoRead option synchronization, and deterministic release on overflow or
  close.
- `handler/pcap`: pipeline-level libpcap capture for inbound and outbound
  payloads without taking message ownership.
- `handler/proxy`: HTTP CONNECT and SOCKS5 client handlers plus SOCKS5
  no-auth, username/password auth, CONNECT/BIND/UDP ASSOCIATE helpers, and
  HAProxy v1/v2 wire helpers for proxy negotiation and source-address
  metadata.
- `handler/metrics` and `observability`: vendor-neutral Channel metrics
  contracts, an atomic low-overhead recorder, a Pipeline handler for lifecycle,
  read/write byte, flush, close, and exception counters, plus Prometheus text
  export and an OpenTelemetry adapter.
- `protocol`: transport-neutral application protocol assembly API for
  request-response flows over stream, datagram, raw packet, and L2 frame
  channels. Client-side `ChannelExchanger` and server-side `Server` use
  `bootstrap.Dialer`/`ServerBootstrap` with explicit adapters instead of
  bypassing the Pipeline.
- `queue`: bounded CAS-based MPSC ring queue for cross-EventLoop delivery.
- `resolver/dns`: Go-native DNS resolver with system fallback, explicit
  exchanger hooks, UDP query support, TCP fallback, hosts override,
  search-domain/ndots expansion, bounded CNAME follow, cache clearing, and
  A/AAAA lookup helpers.
- `resolver/dns/native/macos`: macOS system DNS configuration provider backed
  by `scutil --dns`, mapping native nameserver/search-domain snapshots into
  the Go-native resolver.
- `resolver/dns/quic`: DNS-over-QUIC exchanger using RFC 9250 ALPN `doq`,
  default port `853`, and length-prefixed DNS messages over QUIC streams.
- `timer`: local hashed wheel timer for idle and heartbeat checks.
- `transport`: EventLoop, Channel identity contracts, and a thin factory over
  split poller packages.
- `bootstrap`: Netty-style `ServerBootstrap`, Boss/Worker `EventLoopGroup`,
  `ChildHandler`, and pluggable server transport binding.
- `transport/tcp`: native TCP lifecycle backed by platform socket APIs:
  Linux `socket/bind/listen/accept4`, macOS/BSD `socket/bind/listen/accept`,
  and Windows `WSASocket/bind/listen` with IOCP `AcceptEx` request support.
  TCP channels inject `transport/zerocopy` as the default `FileRegion` writer.
  Linux/macOS/BSD can enable `SO_REUSEPORT` to create one listen socket per
  Boss `EventLoop`.
- `transport/udp`: native datagram transport with server endpoints, connected
  client `Dialer` endpoints, typed `Datagram` messages, default remote address
  handling, platform sockets, and completion backend integration.
- `transport/raw`: custom IP protocol transport with server endpoints,
  connected client `Dialer` endpoints, typed `Packet` messages, protocol
  defaults, and explicit elevated-permission runtime boundaries.
- `transport/sctp`: Linux SCTP one-to-one stream socket transport for
  `ServerBootstrap` and `Dialer`; non-Linux platforms and completion pollers
  return explicit unsupported errors.
- `transport/local`: in-process local transport with paired client/server child
  `Channel` pipelines, Bootstrap/Dialer integration, ChannelOption/Attribute
  application, ByteBuf ownership transfer, read-complete events, close
  propagation, and outbound watermark reporting.
- `transport/udt`: UDT transport extension boundary for pluggable external
  drivers. The core package preserves Bootstrap/Dialer contracts and returns a
  deterministic unsupported error when no driver is supplied.
- `transport/rxtx`: RXTX/serial client transport extension boundary with
  normalized serial settings and a pluggable driver contract; the core package
  does not bind platform serial libraries.
- `transport/zerocopy`: Linux/macOS `sendfile` and Windows `TransmitFile`
  `FileRegion` transfer primitive with explicit unsupported and copy-fallback
  boundaries.
- `transport/l2`: data-link transport abstraction with the same
  `ServerBootstrap`/`Dialer` experience as TCP/UDP/raw/QUIC. Linux uses
  native AF_PACKET; macOS/BSD use native BPF; Windows uses an Npcap driver that
  dynamically loads `wpcap.dll` at runtime. `transport/l2/bpf` and
  `transport/l2/npcap` keep the same injectable `Driver` contract for tests and
  custom deployments without cgo.
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
- `transport/quic/application`: reusable QUIC stream/datagram application
  exchangers, including length-prefixed stream request/response framing and
  datagram response matching for protocols such as DNS-over-QUIC.
- `transport/http3`: HTTP/3 transport binding that maps RFC9000 QUIC request,
  control, and QPACK streams into gnalloy `Channel` pipelines with stream
  lifecycle and byte-count session snapshots.
- `transport/webtransport`: WebTransport over HTTP/3 session binding with
  CONNECT stream session IDs, bidirectional/unidirectional stream prefixes,
  QUIC datagram mapping, and negotiated capability validation.

Examples:

```bash
go run ./examples/echo -addr :9000 -backend default -workers 4
go run ./examples/length-field -addr :9001 -backend default -workers 4
go run ./examples/line-frame -addr :9003 -backend default -workers 4
go run ./examples/fixed-length -addr :9004 -backend default -workers 4 -frame-length 4
go run ./examples/doq-query -server dns.google:853 -name example.com -type A
go run ./examples/protocol-exchange -transport tcp -addr 127.0.0.1:9000 -message ping
go run ./examples/protocol-exchange -transport udp -addr 127.0.0.1:9002 -message ping
go run ./examples/protocol-exchange -transport raw -addr 127.0.0.1 -raw-protocol 253 -message ping
go run ./examples/protocol-exchange -transport l2 -addr eth0 -payload-hex 00112233445566778899aabb88b570696e67
```

Netty parity:

- Netty 对标总览见 `docs/netty-parity.md`。
- Netty codec 对齐清单见 `docs/netty-codec-parity.md`。
- 常见 Pipeline 装配配方见 `recipes` 子包，覆盖 ByteBuf echo、length-field、HTTP/1、HTTP/2、WebSocket、MQTT 和 HTTP/3 stream 初始化器。
- Transport completion 支持矩阵见 `docs/transport-completion-matrix.md`。
- Benchmark parity 口径见 `docs/benchmark-parity.md`。
- TLS 版本对标矩阵见 `benchmarks/parity/tls-version-matrix.json` 和
  `benchmarks/parity/linux-tls-version-matrix.json`。
- HTTP/3 对标矩阵见 `benchmarks/parity/http3-matrix.json` 和
  `benchmarks/parity/linux-http3-matrix.json`。
- Production runbook 见 `docs/production-runbook.md`。
- Microbenchmark suites 见 `benchmarks/microbench`，并通过
  `go run ./cmd/gnalloy-benchdiff -suite hotpath` 执行上一版本对比。

Verification:

```bash
go test ./...
./scripts/verify-regression.sh
./scripts/verify-protocol.sh
ALLOW_SKIP=1 ./scripts/verify-privileged.sh
./scripts/verify-soak.sh
./scripts/verify-bench.sh
go run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/tcp-matrix.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/http3-matrix.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/tls-version-matrix.json
GROUPS=codec,queue,timer ./scripts/verify-bench.sh
go run ./cmd/gnalloy-benchdiff -list-suites
go run ./cmd/gnalloy-benchdiff -base HEAD~1 -suite hotpath -count 5 -benchtime 500ms
```

PowerShell:

```powershell
go test ./...
.\scripts\verify-regression.ps1
.\scripts\verify-protocol.ps1 -ReportPath protocol-report.json
.\scripts\verify-privileged.ps1 -AllowSkip -ReportPath privileged-report.json
.\scripts\verify-soak.ps1 -ReportPath soak-report.json
.\scripts\verify-bench.ps1
.\scripts\verify-platform.ps1 -SkipBench -ReportPath platform-report.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/tcp-matrix.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/windows-tcp.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/http3-matrix.json
go run ./examples/parity-bench -dry-run -config benchmarks/parity/tls-version-matrix.json
.\scripts\verify-bench.ps1 -Groups codec,queue,timer
go run ./cmd/gnalloy-benchdiff -list-suites
go run ./cmd/gnalloy-benchdiff -base HEAD~1 -suite hotpath -count 5 -benchtime 500ms
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
- `scripts/verify-soak.*`: repeats `examples/stress-check` for at least one cycle
  by default; set `GNALLOY_SOAK_DURATION_SECONDS` or `-DurationSeconds` for an
  explicit long-running stability gate.

Platform helper scripts:

```powershell
.\scripts\verify-platform.ps1
.\scripts\verify-platform.ps1 -SkipBench -ReportPath platform-report.json
.\scripts\verify-protocol.ps1 -SkipExternal -ReportPath protocol-report.json
.\scripts\verify-privileged.ps1 -AllowSkip -ReportPath privileged-report.json
.\scripts\verify-soak.ps1 -DurationSeconds 300 -ReportPath soak-report.json
.\scripts\verify-smoke.ps1 -Backend default -Workers 2
.\scripts\verify-stress.ps1 -Backend iocp -Workers 2
.\scripts\verify-iocp.ps1
```

```bash
./scripts/verify-smoke.sh
./scripts/verify-stress.sh
./scripts/verify-protocol.sh
ALLOW_SKIP=1 ./scripts/verify-privileged.sh
GNALLOY_SOAK_DURATION_SECONDS=300 ./scripts/verify-soak.sh
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
- Raw IP and L2 runtime tests are intentionally permission-sensitive. Raw
  sockets usually require administrator privileges or `CAP_NET_RAW`; Linux L2
  AF_PACKET requires the same class of privilege, while BPF/Npcap runtime
  validation requires a native host with the BPF device or Npcap runtime
  installed and an explicit interface selected.
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
- L2 platform integrations must enter through `transport/l2.Driver`; BPF/Npcap
  native details stay isolated under `transport/l2/internal/nativeframe`, with
  no cgo or link-time dependency on vendor libraries.
- Outbound writes are queued per Channel; partial writes remain in the outbound
  buffer until a write-ready/completion event drains them.
- Outbound buffers are gathered across queued `ByteBuf` instances. Readiness
  transports use `writev`/multi-buffer `WSASend`, while completion transports
  can submit batched buffers through io_uring `WRITEV` or IOCP `WSASend`.
- `handler/flow` is intentionally a Pipeline-level inbound gate. It gives
  business handlers Netty-style pause/resume behavior without touching fd
  readiness directly; transport-level AutoRead integration remains controlled
  by each native endpoint.
- io_uring supports optional registered buffers and multishot accept at the
  poller layer; both are opt-in because kernel/version support differs.
- Performance-only CPU affinity is configured on `EventLoopGroup`; Linux binds
  via `runtime.LockOSThread` and `x/sys/unix.SchedSetaffinity`, while unsupported
  platforms fail only when affinity is explicitly requested.
- TCP servers track active child channels so shutdown can drain connections
  before closing off-heap allocators.
