// Package codec 提供可组合到 ChannelPipeline 的编解码 Handler。
//
// 当前稳定公共面是 LengthFieldBasedFrameDecoder。它按长度字段从 TCP 字节流中切出完整帧，
// 并通过 ByteBuf.Slice 传递零拷贝视图。下游 Handler 负责在消费完帧后 Release。
package codec
