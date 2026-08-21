# gnalloy

`gnalloy` is a Go-native, Netty-inspired network framework foundation.

The first implementation slice provides stable contracts and hot-path building
blocks:

- `buffer`: reference-counted `ByteBuf`, zero-copy slices, composite buffers,
  heap allocator, and Linux mmap slab allocator.
- `codec`: zero-copy `LengthFieldBasedFrameDecoder`.
- `channel`: inbound/outbound pipeline contracts and the `Unsafe` bridge that
  normalizes readiness/completion events before they enter business handlers.
- `queue`: bounded CAS-based MPSC ring queue for cross-EventLoop delivery.
- `timer`: local hashed wheel timer for idle and heartbeat checks.
- `transport`: EventLoop, Channel identity contracts, and a thin factory over
  split poller packages.
- `transport/poller`: backend-neutral Poller API, ready for future extraction.
- `transport/poller/epoll`: Linux epoll ET readiness backend.
- `transport/poller/iouring`: Linux io_uring completion backend.
- `transport/poller/kqueue`: macOS/BSD kqueue readiness backend.
- `transport/poller/iocp`: Windows IOCP completion backend.

Design rules:

- A `Channel` is owned by exactly one `EventLoop`.
- Public handlers never see file descriptors or transport-specific events.
- Readiness transports such as epoll and completion transports such as
  io_uring are normalized at `channel.Unsafe`, not in the business pipeline.
- `ByteBuf` slices retain their parent memory and never copy payload bytes.
- Cross-thread operations are submitted to the owner loop.
- Native system calls go through `golang.org/x/sys/unix` or
  `golang.org/x/sys/windows`.
