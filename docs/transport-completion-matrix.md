# gnalloy Transport Completion Matrix

本文档记录 transport 层 readiness/completion 后端的当前事实边界。目标是避免把“可编译”“有包引擎”误写成完整协议栈能力。

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
| L2 frame transport | Linux AF_PACKET native driver; macOS/BSD BPF native driver; Windows Npcap native driver | driver-provided | `transport/l2` exposes `ServerBootstrap`/`Dialer` over injectable `Driver`/`Endpoint` contracts. BPF/Npcap native details live in core under `transport/l2/internal/nativeframe`; Npcap uses runtime DLL loading and no cgo. |
| QUIC packet/runtime | inherits UDP | inherits UDP | `transport/quic` implements packet/header/frame parsing, DCID routing, frame decoder, default runtime handler, ACK range tracking, packet-threshold loss detection, Reno-style congestion window, stream flow-control state, and path validation/migration state. |
| QUIC RFC9000 connection | system UDP socket | system UDP socket | `transport/quic/rfc9000` exposes a complete RFC 9000 QUIC v1 connection adapter backed by TLS 1.3 packet protection, ALPN, 0-RTT opt-in path, bidirectional streams, unidirectional streams, datagrams, WebTransport prerequisites, and connection state. |
| QUIC application assembly | RFC9000 stream/datagram API | RFC9000 stream/datagram API | `transport/quic/application` provides reusable stream request/response and datagram request/response exchangers for protocols such as DNS-over-QUIC. |
| HTTP/3 transport binding | RFC9000 stream API | RFC9000 stream API | `transport/http3` maps request, local/remote control, and QPACK streams into gnalloy `Channel` pipelines using `codec/http3` initializers. |
| WebTransport session binding | RFC9000 stream/datagram API | RFC9000 stream/datagram API | `transport/webtransport` binds an established HTTP/3 CONNECT stream to WebTransport stream channels and HTTP Datagram payload routing. |

## Validation Rules

- Do not claim a transport is production-complete from cross compilation alone.
- UDP completion support is validated through `transport/udp` integration tests because it exercises real socket bind, datagram read, write, and pipeline echo.
- raw integration is not a default gate because it requires elevated privileges on common platforms.
- L2 core integration is validated with an injected fake driver, Linux AF_PACKET cross compilation, BPF/Npcap package tests, and platform matrix metadata. `scripts/verify-privileged.*` opens Linux raw sockets, AF_PACKET endpoints, BPF endpoints, and Npcap endpoints on native hosts when the corresponding interface environment variable is set.
- QUIC packet/runtime validation covers packet parsing/routing, frame decoder, runtime handler, ACK/loss/congestion, stream state, and path state unit tests.
- DNS-over-QUIC validation uses `resolver/dns/quic` unit tests for ALPN/default-port behavior and QUIC application length-prefixed framing. External DoQ server tests are an opt-in runtime gate.
- `scripts/verify-protocol.*` validates protocol assembly packages, dry-runs DoQ and stream/datagram/raw/L2 unified examples, validates the parity benchmark spec, and optionally runs an external DoQ query through `GNALLOY_DOQ_ADDR`.
- `protocol.Server` provides the server-side counterpart to `protocol.ChannelExchanger`, preserving UDP source addresses, raw IP protocol metadata, and L2 frame metadata through explicit adapters.
- `scripts/verify-soak.*` repeats `examples/stress-check` as a stability gate; the default is one short cycle, while `GNALLOY_SOAK_DURATION_SECONDS` or `-DurationSeconds` turns it into a long-running soak.
- QUIC RFC9000 validation covers localhost encrypted TLS 1.3 interop by default. External stack interop is available through `GNALLOY_QUIC_INTEROP_ADDR`, `GNALLOY_QUIC_INTEROP_ALPN`, `GNALLOY_QUIC_INTEROP_SERVER_NAME`, `GNALLOY_QUIC_INTEROP_INSECURE`, `GNALLOY_QUIC_INTEROP_PAYLOAD`, and `GNALLOY_QUIC_INTEROP_EXPECT`.
- `scripts/platform-matrix.json` is the machine-readable platform gate source; `validation/platformmatrix` validates its schema and duplicate target boundaries.
- `scripts/verify-platform.ps1 -SkipBench -ReportPath platform-report.json` records native tests, cross compilation, skipped benchmarks, and source scan as `passed`/`skipped`/`failed`.
- `.github/workflows/validation.yml` runs `go test ./...` on Ubuntu, macOS, and Windows, and runs the Windows platform matrix gate with a report artifact.
