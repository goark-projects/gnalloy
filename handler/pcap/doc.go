// Package pcap 提供 Pipeline 级 libpcap 捕获 Handler。
//
// Handler 只观察 ByteBuf、UDP/raw typed message、[]byte 和 string 的 payload，
// 不接管消息所有权。捕获数据使用 LINKTYPE_USER0 作为默认链路类型，便于业务
// 在排障或压测时保存自定义协议字节流。
package pcap
