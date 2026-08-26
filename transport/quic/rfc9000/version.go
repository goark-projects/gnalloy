package rfc9000

import enginequic "goark.dev/gnalloy/transport/quic"

// Version 是 RFC9000 适配层公开的 QUIC 版本号类型。
type Version = enginequic.Version

// Version1 是 RFC 9000 定义的 QUIC v1。
const Version1 Version = enginequic.Version1
