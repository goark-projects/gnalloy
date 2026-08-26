// Package flush 提供 Netty FlushConsolidationHandler 风格的 flush 聚合处理器。
//
// 该包只合并出站 flush 事件，不缓存写入消息；写入仍由 pipeline 中的其他出站
// handler 或 transport sink 管理。
package flush
