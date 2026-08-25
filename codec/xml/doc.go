// Package xml 提供 XML 文档切帧与 token 编解码。
//
// FrameDecoder 按完整 XML document 输出 ByteBuf；TokenDecoder 将完整 XML
// document 解成 Go 化 token 事件，便于业务 pipeline 消费。
package xml
