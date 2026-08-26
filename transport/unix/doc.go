// Package unix 提供 Unix domain socket 传输实现。
//
// Unix domain socket 是本机 IPC 传输，不具备 TCP 的 IP、端口、Nagle 等语义。本包
// 复用 gnalloy 的 Bootstrap、EventLoop、Channel 和 Pipeline 契约，只把底层地址和
// socket 生命周期替换为 AF_UNIX。
package unix
