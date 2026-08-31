# gnalloy Transport Completion Matrix

本文档记录 transport 层 readiness/completion 后端的当前事实边界。目标是避免把“可编译”“有包装门面”误写成完整协议栈能力。

## Backend Model

| Backend | Platform | Model | Current scope |
| --- | --- | --- | --- |
| `epoll` | Linux | readiness | TCP/UDP/raw socket readiness |
| `kqueue` | macOS/BSD | readiness | TCP/UDP/raw socket readiness |
| `std` | all | readiness | polling fallback |
| `iouring` | Linux | completion | TCP accept/read/write/close, UDP datagram read/write |
| `iocp` | Windows | completion | TCP AcceptEx/read/write/close, UDP datagram read/write |
| `memory` | all | completion | in-process tests only |

## Transport Scope

| Transport | Readiness backends | Completion backends | Notes |
| --- | --- | --- | --- |
| TCP | `epoll`, `kqueue`, `std` | `iouring`, `iocp`, `memory` | Completion path covers accept, connected read/write, close, and existing write-buffer backpressure. |
| UDP | `epoll`, `kqueue`, `std` | `iouring`, `iocp`, `memory` | Completion path uses datagram `IORequest` and is covered by the UDP echo integration test on the platform default backend. |
| raw | `epoll`, `kqueue`, `std` where OS raw socket is available | `iouring`, `iocp`, `memory` where raw socket and datagram completion are available | Runtime success still depends on administrator or `CAP_NET_RAW` permission and OS raw-socket policy. |
| Unix domain socket | Linux `epoll`/`std`; macOS/BSD `kqueue`/`std` | unsupported | Stream sockets integrate with `ServerBootstrap`/`Dialer`; datagram endpoints expose AF_UNIX datagram send/receive. Linux additionally exposes SO_PEERCRED peer credentials and SCM_RIGHTS file-descriptor passing. |
| SCTP | Linux `epoll` and `std` when kernel SCTP is available | unsupported | `transport/sctp` exposes one-to-one stream sockets through `ServerBootstrap`/`Dialer`; runtime validation checks platform, numeric address, config, every boss/worker/client poller, and kernel SCTP socket availability before opening SCTP sockets. Linux sockets apply `SCTP_INITMSG` and `SCTP_NODELAY`; non-Linux platforms and completion pollers fail fast with explicit unsupported errors. |
| local in-VM | in-process | in-process | `transport/local` pairs client and server child `Channel` pipelines without OS fd registration; EventLoop ownership, read-complete events, close propagation, and ByteBuf ownership are covered by unit tests. |
| L2 frame transport | Linux AF_PACKET native driver; macOS/BSD BPF native driver; Windows Npcap native driver | driver-provided | `transport/l2` exposes `ServerBootstrap`/`Dialer` over injectable `Driver`/`Endpoint` contracts. BPF/Npcap native details live in core under `transport/l2/internal/nativeframe`; Npcap uses runtime DLL loading and no cgo. |
| QUIC RFC9000 connection | system UDP socket | system UDP socket | `transport/quic` exposes the concrete RFC 9000 QUIC v1 connection adapter backed by quic-go, TLS 1.3 packet protection, ALPN, 0-RTT opt-in path, bidirectional streams, unidirectional streams, datagrams, WebTransport prerequisites, connection state/stats, qlog writer factory, and native engine capability snapshots. No separate QUIC provider subpackage is exposed because quic-go is the single production engine. |
| QUIC application assembly | RFC9000 stream/datagram API | RFC9000 stream/datagram API | `transport/quic/application` provides reusable stream request/response and datagram request/response exchangers for protocols such as DNS-over-QUIC. |
| HTTP/3 transport binding | RFC9000 stream API | RFC9000 stream API | `transport/http3` maps request, local/remote control, QPACK, and push streams into gnalloy `Channel` pipelines using `codec/http3` initializers, including push ID prefix read/write and `OpenPushStream`/`AcceptPushStream` session helpers. |
| WebTransport session binding | RFC9000 stream/datagram API | RFC9000 stream/datagram API | `transport/webtransport` binds an established HTTP/3 CONNECT stream to WebTransport stream channels and HTTP Datagram payload routing. |

## Validation Rules

- Do not claim a transport is production-complete from cross compilation alone.
- UDP completion support is validated through `transport/udp` integration tests because it exercises real socket bind, datagram read, write, and pipeline echo.
- raw integration is not a default gate because it requires elevated privileges on common platforms.
- Unix domain socket validation covers non-Unix unsupported errors on Windows and compile-time Linux syscall compatibility by default; native datagram/fd-passing runtime tests must run on a Linux host.
- SCTP validation covers startup capability snapshots, kernel socket probing, invalid config/address checks, readiness-only poller enforcement, `SCTP_INITMSG`, and `SCTP_NODELAY` option mapping. Linux native smoke still requires a host with kernel SCTP support.
- local transport validation is a unit-test gate because it intentionally has no native fd, kernel readiness, or completion backend.
- UDT/RXTX are intentionally excluded from the transport matrix because Netty has removed or stopped maintaining those transports; gnalloy does not keep core adapter packages for them.
- L2 core integration is validated with an injected fake driver, Linux AF_PACKET cross compilation, BPF/Npcap package tests, and platform matrix metadata. `scripts/verify-privileged.*` opens Linux raw sockets, AF_PACKET endpoints, BPF endpoints, and Npcap endpoints on native hosts when the corresponding interface environment variable is set.
- DNS-over-QUIC validation uses `resolver/dns/quic` unit tests for ALPN/default-port behavior and QUIC application length-prefixed framing. External DoQ server tests are an opt-in runtime gate.
- `scripts/verify-protocol.*` validates protocol assembly packages, dry-runs DoQ and stream/datagram/raw/L2 unified examples, validates the parity benchmark spec, and optionally runs an external DoQ query through `GNALLOY_DOQ_ADDR`.
- `protocol.Server` provides the server-side counterpart to `protocol.ChannelExchanger`, preserving UDP source addresses, raw IP protocol metadata, and L2 frame metadata through explicit adapters.
- `scripts/verify-soak.*` repeats `examples/stress-check` as a stability gate; the default is one short cycle, while `GNALLOY_SOAK_DURATION_SECONDS` or `-DurationSeconds` turns it into a long-running soak.
- QUIC validation covers localhost encrypted TLS 1.3 interop, connection stats mapping, qlog writer factory wiring, and native engine capability snapshots by default. External stack interop is available through `GNALLOY_QUIC_INTEROP_ADDR`, `GNALLOY_QUIC_INTEROP_ALPN`, `GNALLOY_QUIC_INTEROP_SERVER_NAME`, `GNALLOY_QUIC_INTEROP_INSECURE`, `GNALLOY_QUIC_INTEROP_PAYLOAD`, and `GNALLOY_QUIC_INTEROP_EXPECT`.
- `scripts/platform-matrix.json` is the machine-readable platform gate source; `validation/platformmatrix` validates its schema and duplicate target boundaries.
- `scripts/verify-platform.ps1 -SkipBench -ReportPath platform-report.json` records native tests, cross compilation, skipped benchmarks, and source scan as `passed`/`skipped`/`failed`.
- `.github/workflows/validation.yml` runs `go test ./...` on Ubuntu, macOS, and Windows, and runs the Windows platform matrix gate with a report artifact.
