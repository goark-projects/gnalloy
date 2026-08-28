// Package flow 提供 Pipeline 入站流控处理器。
//
// Handler 只负责业务层消息暂停、排队、恢复和上限保护，不直接操作底层 fd。
// 底层读兴趣由 ChannelOptionAutoRead 表达，具体传输层是否下沉该选项由 transport 决定。
package flow
