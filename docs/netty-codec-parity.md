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
| `HttpObjectDecoder/Encoder` | `codec/http1` | partial | 已有 HTTP/1.x 基础解析，完整聚合/分块后续扩展。 |
| `WebSocketFrameDecoder/Encoder` | `codec/websocket` | partial | 已有 frame 和握手基础，扩展压缩和控制帧策略后续补齐。 |
| `Http3FrameCodec` | `codec/http3` | done | 支持 HTTP/3 DATA、HEADERS、SETTINGS、PUSH_PROMISE、GOAWAY、MAX_PUSH_ID、PRIORITY_UPDATE 和未知扩展帧。 |
| `Redis RESP` | `codec/redis` | done | Netty 无一一对应核心类，但提供协议帧能力。 |
| `MQTT` | `codec/mqtt` | done | 支持 MQTT 3.1.1/MQTT5 固定头、结构化包、属性、原因码、AUTH 和零拷贝 PUBLISH payload。 |
| `DnsQueryEncoder/DnsResponseDecoder` | `codec/dns`, `resolver/dns` | partial | 已有 DNS wire message 编解码、name 压缩解析、A/AAAA resolver；高级 record 类型后续扩展。 |
| `JsonObjectDecoder` | `codec.JsonObjectDecoder` | done | 按对象/数组边界切帧，不做完整 JSON 语义校验。 |
| `ICMP/IP` | `codec/icmp`, `codec/ip` | done | 为 raw socket 和自定义 IP 协议提供基础帧。 |

## 延后或独立扩展

| Netty 模块 | 状态 | 原因 |
| --- | --- | --- |
| compression (`brotli`, `snappy`, `lz4`) | defer | 涉及外部算法依赖，应拆成独立扩展包。 |
| serialization/marshalling | skip | Go 生态不应在核心网络层绑定对象序列化框架。 |
| socks/smtp/stomp | planned | 可独立协议包实现，不应堵塞核心 I/O 和 frame 层。 |
| TLS/SslHandler | defer | 应先稳定底层 fd 与 `crypto/tls` 或平台 TLS 的边界。 |
| HTTP/2/HTTP/3 | defer | HTTP/3 与 QUIC 状态机强相关，需独立里程碑。 |

## 热路径约束

- 入站 frame decoder 默认使用 `CompositeCumulator`，跨 buffer 切片不复制 payload。
- 需要连续内存的 codec 可以切换 `MergeCumulator`，付出一次复制换取单段可读视图。
- Frame decoder 基准必须持续保持稳态 `0 B/op`、`0 allocs/op`。
- 出站 encoder 接管被处理消息的所有权；写失败时必须释放尚未移交给下游的 `ByteBuf`。
