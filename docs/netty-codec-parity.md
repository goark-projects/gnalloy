# gnalloy Netty Codec 对齐清单

## 目标

`gnalloy/codec` 的目标不是逐字复制 Netty API，而是保留 Netty 使用者熟悉的编解码层次，并按 Go 的接口、显式 `error`、`ByteBuf` 引用计数和零拷贝约束落地。状态含义如下：

- `done`: 已有可用实现，并有单元测试或基准覆盖。
- `partial`: 已有核心路径，仍缺少 Netty 的部分参数、异常语义或协议边界。
- `planned`: 应实现，且适合无外部依赖的 Go 化版本。
- `defer`: 需要 ASN.1、序列化框架等额外依赖或较大协议面，后续独立设计。
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
| `JdkZlibEncoder/JdkZlibDecoder`、brotli/snappy/lz4/zstd/bzip2/lzma/FastLZ/LZF compression codecs | `codec/compression`、`codec/compression/brotli`、`codec/compression/snappy`、`codec/compression/lz4`、`codec/compression/zstd`、`codec/compression/bzip2`、`codec/compression/lzma`、`codec/compression/fastlz`、`codec/compression/lzf` | done | 根包支持标准库 gzip/zlib；扩展子包支持 Brotli、Snappy stream、Netty `SnappyFrame*`/`SnappyFramed*` dialect、LZ4 stream、Zstandard、BZip2、classic LZMA、Netty FastLZ frame 和 Netty LZF chunk，解码器提供最大解压大小限制。 |
| `ByteBufUtil` | `buffer` 工具函数 | done | 覆盖 hex dump、equals、compare、index 等基础能力。 |

## 协议 codec

| Netty 类型 | gnalloy 对应 | 状态 | 说明 |
| --- | --- | --- | --- |
| `ProtobufEncoder` / `ProtobufDecoder` | `codec/protobuf.Encoder` / `codec/protobuf.Decoder` | done | 支持 `proto.Message` 对象到 `ByteBuf` 编码、显式 factory 解码、空消息保留、碎片输入按需复制和最大消息大小限制。 |
| `ProtobufVarint32FrameDecoder` | `codec/protobuf.ProtobufVarint32FrameDecoder` | done | 支持跨 buffer varint header、超长帧、非法 varint。 |
| `ProtobufVarint32LengthFieldPrepender` | `codec/protobuf.ProtobufVarint32LengthFieldPrepender` | done | 出站 varint32 header + payload 零拷贝。 |
| `HAProxyMessageDecoder/Encoder` | `codec/haproxy` | done | 支持 PROXY protocol v1/v2、TCP/UDP IPv4/IPv6、UNIX 地址和 TLV。 |
| `SpdyFrameDecoder/Encoder` | `codec/spdy` | done | 支持 SPDY/3 data、SYN、RST、SETTINGS、PING、GOAWAY、HEADERS、WINDOW_UPDATE 和未知控制帧。 |
| `HttpObjectDecoder/Encoder`、`QueryStringDecoder/Encoder`、`HttpContentCompressor/Decompressor`、`HttpClientUpgradeHandler/HttpServerUpgradeHandler` helpers | `codec/http1` | done | 支持请求/响应、Content-Length、chunked、trailers、聚合器、100-continue、chunked 出站、query string 有序参数、gzip/deflate full-message content 编解码和 Upgrade 请求/101 响应 helper。 |
| `ClientCookieEncoder/Decoder`、`ServerCookieEncoder/Decoder` | `codec/http1/cookie` | done | 支持 Cookie 请求多值、Set-Cookie 响应属性、Expires、Max-Age、SameSite、Secure、HttpOnly、Partitioned 和 Append 热路径。 |
| `HttpPostRequestDecoder/Encoder` | `codec/http1/multipart` | done | Go 化 boundary 解析、受限流式 part 消费、ByteBuf/Request 适配和 append-style form-data 输出。 |
| `WebSocketFrameDecoder/Encoder` | `codec/websocket` | done | 支持握手、mask policy、控制帧、close 握手、UTF-8 校验、fragment 聚合和 idle ping/close。 |
| `PerMessageDeflate*ExtensionHandshaker`、`WebSocketClient/ServerCompressionHandler` | `codec/websocket/deflate` | done | 支持 permessage-deflate 参数解析、RSV1 承载、无上下文复用压缩/解压、分片最终聚合和解压大小限制。 |
| `Http2FrameCodec` / `Http2ConnectionHandler` helpers | `codec/http2`、`codec/http2/h2c`、`codec/http2/http1bridge`、`codec/http2/defense`、`codec/http2/chunked` | done | 支持 HTTP/2 通用帧和 DATA/HEADERS/SETTINGS/PING/GOAWAY 等 typed frame、client preface、SETTINGS ACK、连接级 SETTINGS/GOAWAY/receive-window controller、h2c HTTP2-Settings header、HPACK header block 编解码、完整 stream-frame 到 HTTP 对象桥接、RST flood 防御、control frame 写入上限和 chunked DATA 输入。 |
| `Http2MultiplexHandler` / `DefaultHttp2RemoteFlowController` / `StreamBufferingEncoder` / `WeightedFairQueueByteDistributor` helper | `codec/http2.StreamMultiplexer` / `StreamChildChannel` / `OutboundFlowController` / `StreamBufferingEncoder` / `codec/http2/scheduler` | done | 支持 stream 生命周期事件、奇偶性校验、END_STREAM 半关闭、RST/GOAWAY、连接/stream 窗口校验、SETTINGS_INITIAL_WINDOW_SIZE 动态调整、出站 DATA 有界排队、remote max concurrent stream 下的出站 stream 缓冲和按权重分配发送预算。 |
| `Http3FrameCodec` / `Http3FrameToHttpObjectCodec` / push stream helpers | `codec/http3`、`codec/http3/http1bridge`、`transport/http3` | done | 支持 HTTP/3 DATA、HEADERS、SETTINGS、PUSH_PROMISE、GOAWAY、MAX_PUSH_ID、PRIORITY_UPDATE、未知扩展帧、QPACK header block、HTTP 对象桥接、push stream ID 前缀、push stream 初始化/校验/manager、control stream 顺序校验、连接级 SETTINGS/GOAWAY/server push 状态管理、QUIC 单向 stream type 前缀、生命周期事件和低基数 frame stats。 |
| `SctpInboundByteStreamHandler` / `SctpOutboundByteStreamHandler` / `SctpMessageCompletionHandler` | `codec/sctp` | done | 支持 SCTP stream 元数据、按 protocol/stream 的 ByteBuf 适配和分片聚合；payload 通过 `ByteBuf` 引用计数传递，避免额外复制。 |
| `BinaryMemcache*` | `codec/memcache` | done | 支持 Memcached binary request/response frame、Full request/response 对象、零拷贝对象聚合、client/server 方向 codec helper、extras/key/value、opaque 和 CAS。 |
| `Redis RESP` | `codec/redis` | done | Netty 无一一对应核心类，但提供协议帧和值解码能力；已覆盖 fuzz smoke。 |
| `MQTT` | `codec/mqtt` | done | 支持 MQTT 3.1.1/MQTT5 固定头、结构化包、属性、原因码、AUTH 和零拷贝 PUBLISH payload。 |
| `DnsQueryEncoder/DnsResponseDecoder` | `codec/dns`, `resolver/dns` | done | 支持 DNS wire message、name 压缩、A/AAAA/NS/CNAME/PTR/MX/TXT/SRV/SOA/OPT 常用记录和 resolver；已覆盖 fuzz smoke。 |
| `SmtpRequest/ResponseEncoder/Decoder` | `codec/smtp` | done | 支持 SMTP request、multiline response 和 DATA dot-stuffing。 |
| `Socks4/Socks5` | `codec/socks` | done | 支持 SOCKS4a、SOCKS5 greeting、method selection、RFC1929 username/password auth、private auth response、command request/reply 和 IPv4/domain/IPv6 地址。 |
| `StompSubframeDecoder/Encoder` | `codec/stomp` | done | 支持 STOMP 1.2 frame、heartbeat、header escape、content-length 和 NUL body。 |
| `RtspRequest/ResponseDecoder/Encoder` | `codec/rtsp` | done | 支持 RTSP/1.0 请求、响应、header 和 Content-Length body。 |
| `XmlFrameDecoder/XmlDecoder` | `codec/xml` | done | 支持完整 XML document 切帧和 Go 化 token 流。 |
| `JsonObjectDecoder` | `codec.JsonObjectDecoder` | done | 按对象/数组边界切帧，不做完整 JSON 语义校验。 |
| `ICMP/IP` | `codec/icmp`, `codec/ip` | done | 为 raw socket 和自定义 IP 协议提供基础帧。 |
| `SslHandler` | `handler/tls`、`handler/tls/provider/standard` | done | 默认基于 `crypto/tls` 的 pipeline TLS handler，并提供标准库 provider 子包、handler 级 TLS1.3/ALPN/SNI 能力校验、握手事件和 stapled OCSP required/event/validator 边界；QUIC packet protection 由 `transport/quic` 的 quic-go 引擎能力快照独立评估。 |
| `SniHandler` / `StartTls` / `OptionalSslHandler` | `handler/tls` | done | 支持 StartTLS 事件启动、SNI 配置选择、ClientHello SNI/ALPN/cipher/version inspection、Optional TLS 探测事件、握手事件和握手后主机名校验。 |

## 延后或独立扩展

| Netty 模块 | 状态 | 原因 |
| --- | --- | --- |
| compression (`brotli`, `snappy`, `lz4`, `zstd`, `bzip2`, `lzma`, `fastlz`, `lzf`) | done | 已拆到独立 `codec/compression/*` 子包，Snappy 同时覆盖 stream 与 Netty framed dialect，避免根包直接绑定外部算法依赖或 JVM Unsafe 选择项。 |
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
| DNS/Redis/HTTP2/HTTP3 | `FuzzDNSParseMessage`、`FuzzRedisFramePipeline`、`FuzzHTTP2FramePipeline`、`FuzzHTTP3FramePipeline` | 覆盖 P2 新增的 Netty 常用协议 smoke；HTTP/2 对象桥接、调度、防御、chunked DATA 和 HTTP/3 对象桥接、push stream 另有 focused unit tests。 |
| QUIC | `TestListenDialAddrEchoOverRFC9000QUIC`、`TestFacadeExposesQUICGoBackedTransport` | 覆盖 quic-go-backed RFC9000/TLS1.3 本机互通连接栈和 Gnalloy QUIC 门面边界。 |
