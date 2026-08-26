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
| QUIC | inherits UDP | inherits UDP | Current package is a QUIC packet/runtime engine over UDP. It implements packet/header/frame parsing, DCID routing, ACK range tracking, packet-threshold loss detection, Reno-style congestion window, stream flow-control state, and path validation/migration state. It does not yet implement full TLS 1.3 packet protection, handshake, retransmission timer, 0-RTT, or RFC 9000 interop as a complete connection stack. |

## Validation Rules

- Do not claim a transport is production-complete from cross compilation alone.
- UDP completion support is validated through `transport/udp` integration tests because it exercises real socket bind, datagram read, write, and pipeline echo.
- raw integration is not a default gate because it requires elevated privileges on common platforms.
- QUIC transport validation covers packet parsing/routing plus runtime state unit tests. Full production validation still requires interop tests against an external QUIC stack and encrypted TLS 1.3 packet protection.
