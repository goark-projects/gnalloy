// Package parity 提供外部框架对标压测报告工具。
//
// 该包不内置任何“超过 Netty”的结论，只负责在同一台机器上执行用户声明的
// gnalloy、Netty、gnet、netpoll 等命令，并把机器信息、命令、耗时和原始输出写入
// 可复现报告。
package parity
