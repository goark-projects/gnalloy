// Package smtp 提供 SMTP 请求、响应和 DATA 内容编解码。
//
// ResponseDecoder 支持多行响应聚合；RequestEncoder 和 ResponseEncoder 面向
// 控制行；DataEncoder 负责 DATA 阶段的点转义和结束行。
package smtp
