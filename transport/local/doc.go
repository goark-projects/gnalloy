// Package local 提供进程内本地传输，适用于同进程组件、测试和嵌入式协议装配。
//
// local transport 不接触操作系统 fd；它通过成对 Channel 连接 client 和 server
// child pipeline，保留 Bootstrap/Dialer、ChannelOption、Attribute 和 ByteBuf
// 所有权语义。
package local
