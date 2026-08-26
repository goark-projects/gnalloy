# gnalloy Netty Codec 对齐清单

## 目标

`gnalloy/codec` 的目标不是逐字复制 Netty API，而是保留 Netty 使用者熟悉的编解码层次，并按 Go 的接口、显式 `error`、`ByteBuf` 引用计数和零拷贝约束落地。状态含义如下：

- `done`: 已有可用实现，并有单元测试或基准覆盖。
- `partial`: 已有核心路径，仍缺少 Netty 的部分参数、异常语义或协议边界。
- `planned`: 应实现，且适合无外部依赖的 Go 化版本。
- `defer`: 需要压缩、TLS、ASN.1、序列化框架等额外依赖或较大协议面，后续独立设计。
- `skip`: 不适合 gnalloy 核心，应该由业务或独立扩展包承担。

## 基础模板

| Netty 类型 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ByteToMessageDecoder` | `codec.ByteToMessageDecoder` | done | 支持半包累积、循环解码、无进展保护、显式释放。 |
| `MessageToByteEncoder` | `codec.MessageToByteEncoder` | done | 出站消息编码为 `ByteBuf`。 |
| `MessageToMessageDecoder` | `codec.MessageToMessageDecoder` | done | 入站消息到消息转换。 |
| `MessageToMessageEncoder` | `codec.MessageToMessageEncoder` | done | 出站消息到消息转换。 |
| `ByteToMessageCodec` | `codec.ByteToMessageCodec` | done | 入站字节解码 + 出站字节编码组合。 |
| `MessageToMessageCodec` | `codec.MessageToMessageCodec` | done | 入站/出站消息转换组合。 |
| `CombinedChannelDuplexHandler` | `codec.CombinedChannelDuplexHandler` | done | 组合 inbound/outbound handler。 |
| `ReplayingDecoder` | `codec.ReplayingDecoder` | done | Go 化为显式 `ErrReplayNeedMore`，不使用异常回放。 |

## 字节流帧编解码

| Netty 类型 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `LengthFieldBasedFrameDecoder` | `codec.LengthFieldBasedFrameDecoder` | done | 支持字段偏移、字段长度、补偿、剥离、字节序、fail-fast/fail-slow。 |
| `LengthFieldPrepender` | `codec.LengthFieldPrepender` | done | 出站 header 与 payload 分离写，避免复制 payload。 |
| `LineBasedFrameDecoder` | `codec.LineBasedFrameDecoder` | done | 支持 `\n`/`\r\n`、剥离分隔符、fail-fast/fail-slow。 |
| `LineEncoder` | `codec.LineEncoder` | done | 支持 string、`[]byte`、`ByteBuf`，ByteBuf payload 零拷贝。 |
| `DelimiterBasedFrameDecoder` | `codec.DelimiterBasedFrameDecoder` | done | 多分隔符取最短帧。 |
| `DelimiterBasedFrameEncoder` | `codec.DelimiterBasedFrameEncoder` | done | 出站追加自定义分隔符。 |
| `FixedLengthFrameDecoder` | `codec.FixedLengthFrameDecoder` | done | 固定长度切帧，零拷贝切片。 |
| `FixedLengthFrameEncoder` | `codec.FixedLengthFrameEncoder` | done | 校验固定长度后出站写出。 |
| `TooLongFrameException` | `codec.TooLongFrameError` | done | 通过 `errors.Is(err, ErrFrameTooLong)` 匹配。 |

## 常用转换

| Netty 类型 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ByteArrayDecoder` | `codec.ByteSliceDecoder` | done | 会复制为 Go `[]byte`，极致路径应消费 `ByteBuf`。 |
| `ByteArrayEncoder` | `codec.ByteSliceEncoder` | done | 将 `[]byte` 写入 allocator 分配的 `ByteBuf`。 |
| `StringDecoder` | `codec.StringDecoder` | done | Go string 不可变，必须复制。 |
| `StringEncoder` | `codec.StringEncoder` | done | 使用只读 string view 写入 `ByteBuf`。 |
| `Base64Encoder` | `codec.Base64Encoder` | done | 支持标准和 URL dialect。 |
| `Base64Decoder` | `codec.Base64Decoder` | done | 解码失败走 `ExceptionCaught`。 |
| `JdkZlibEncoder/JdkZlibDecoder` | `codec/compression` | done | 支持 gzip/zlib，解码器提供最大解压大小限制。 |
| `ByteBufUtil` | `buffer` 工具函数 | done | 覆盖 hex dump、equals、compare、index 等基础能力。 |

## 协议 codec

| Netty 类型 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ProtobufVarint32FrameDecoder` | `codec/protobuf.ProtobufVarint32FrameDecoder` | done | 支持跨 buffer varint header、超长帧、非法 varint。 |
| `ProtobufVarint32LengthFieldPrepender` | `codec/protobuf.ProtobufVarint32LengthFieldPrepender` | done | 出站 varint32 header + payload 零拷贝。 |
| `HAProxyMessageDecoder/Encoder` | `codec/haproxy` | done | 支持 PROXY protocol v1/v2、TCP/UDP IPv4/IPv6、UNIX 地址和 TLV。 |
| `SpdyFrameDecoder/Encoder` | `codec/spdy` | done | 支持 SPDY/3 data、SYN、RST、SETTINGS、PING、GOAWAY、HEADERS、WINDOW_UPDATE 和未知控制帧。 |
| `HttpObjectDecoder/Encoder` | `codec/http1` | done | 支持请求/响应、Content-Length、chunked、trailers、聚合器、100-continue 和 chunked 出站。 |
| `WebSocketFrameDecoder/Encoder` | `codec/websocket` | done | 支持握手、mask policy、控制帧、close 握手、UTF-8 校验、fragment 聚合和 idle ping/close。 |
| `Http2FrameCodec` | `codec/http2` | done | 支持 HTTP/2 通用帧和 DATA/HEADERS/SETTINGS/PING/GOAWAY 等 typed frame，HPACK header block 编解码保持独立 handler；已覆盖 fuzz smoke。 |
| `Http2MultiplexHandler` | `codec/http2.StreamMultiplexer` / `StreamChildChannel` | done | 支持 stream 生命周期事件、奇偶性校验、END_STREAM 半关闭、RST/GOAWAY、连接/stream 窗口校验和 Netty 风格 child-channel 入站体验。 |
| `Http3FrameCodec` | `codec/http3` | done | 支持 HTTP/3 DATA、HEADERS、SETTINGS、PUSH_PROMISE、GOAWAY、MAX_PUSH_ID、PRIORITY_UPDATE、未知扩展帧、QPACK header block、control stream 顺序校验和 QUIC 单向 stream type 前缀；已覆盖 fuzz smoke。 |
| `BinaryMemcache*` | `codec/memcache` | done | 支持 Memcached binary request/response header、extras、key、value、opaque 和 CAS。 |
| `Redis RESP` | `codec/redis` | done | Netty 无一一对应核心类，但提供协议帧和值解码能力；已覆盖 fuzz smoke。 |
| `MQTT` | `codec/mqtt` | done | 支持 MQTT 3.1.1/MQTT5 固定头、结构化包、属性、原因码、AUTH 和零拷贝 PUBLISH payload。 |
| `DnsQueryEncoder/DnsResponseDecoder` | `codec/dns`, `resolver/dns` | done | 支持 DNS wire message、name 压缩、A/AAAA/NS/CNAME/PTR/MX/TXT/SRV/SOA/OPT 常用记录和 resolver；已覆盖 fuzz smoke。 |
| `SmtpRequest/ResponseEncoder/Decoder` | `codec/smtp` | done | 支持 SMTP request、multiline response 和 DATA dot-stuffing。 |
| `Socks4/Socks5` | `codec/socks` | done | 支持 SOCKS4a、SOCKS5 greeting、method selection、command request/reply 和 IPv4/domain/IPv6 地址。 |
| `StompSubframeDecoder/Encoder` | `codec/stomp` | done | 支持 STOMP 1.2 frame、heartbeat、header escape、content-length 和 NUL body。 |
| `RtspRequest/ResponseDecoder/Encoder` | `codec/rtsp` | done | 支持 RTSP/1.0 请求、响应、header 和 Content-Length body。 |
| `XmlFrameDecoder/XmlDecoder` | `codec/xml` | done | 支持完整 XML document 切帧和 Go 化 token 流。 |
| `JsonObjectDecoder` | `codec.JsonObjectDecoder` | done | 按对象/数组边界切帧，不做完整 JSON 语义校验。 |
| `ICMP/IP` | `codec/icmp`, `codec/ip` | done | 为 raw socket 和自定义 IP 协议提供基础帧。 |
| `SslHandler` | `handler/tls` | done | 基于 `crypto/tls` 的 pipeline TLS handler，保持和 TCP/QUIC 等传输入口一致。 |
| `SniHandler` / `StartTls` | `handler/tls` | done | 支持 StartTLS 事件启动、SNI 配置选择、握手事件和握手后主机名校验。 |

## 延后或独立扩展

| Netty 模块 | 状态 | 原因 |
| --- | --- | --- |
| compression (`brotli`, `snappy`, `lz4`) | defer | 涉及外部算法依赖，应拆成独立扩展包。 |
| serialization/marshalling | skip | Go 生态不应在核心网络层绑定对象序列化框架。 |

## 热路径约束

- 入站 frame decoder 默认使用 `CompositeCumulator`，跨 buffer 切片不复制 payload。
- 需要连续内存的 codec 可以切换 `MergeCumulator`，付出一次复制换取单段可读视图。
- Frame decoder 基准必须持续保持稳态 `0 B/op`、`0 allocs/op`。
- 出站 encoder 接管被处理消息的所有权；写失败时必须释放尚未移交给下游的 `ByteBuf`。

## Fuzz 覆盖

| 范围 | 入口 | 说明 |
| --- | --- | --- |
| 基础帧 | `FuzzLengthFieldBasedFrameDecoder`、`FuzzLineBasedFrameDecoder`、`FuzzDelimiterBasedFrameDecoder` | 覆盖半包、超长帧和分隔符边界。 |
| HTTP/WebSocket/MQTT | `FuzzHTTP1RequestDecoder`、`FuzzWebSocketFrameDecoder`、`FuzzMQTTFramePipeline` | 覆盖常用应用层协议 pipeline。 |
| DNS/Redis/HTTP2/HTTP3 | `FuzzDNSParseMessage`、`FuzzRedisFramePipeline`、`FuzzHTTP2FramePipeline`、`FuzzHTTP3FramePipeline` | 覆盖 P2 新增的 Netty 常用协议 smoke。 |
| QUIC | `FuzzQUICParseHeaderBytes`、`FuzzQUICFrameScanner`、`TestListenDialAddrEchoOverRFC9000QUIC` | 覆盖 QUIC packet/header/frame 边界，以及 RFC9000/TLS1.3 本机互通连接栈。 |
