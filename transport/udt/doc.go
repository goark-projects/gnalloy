// Package udt 定义 UDT 传输的 gnalloy 扩展边界。
//
// UDT 不属于 Go 标准库，也缺少可长期维护的跨平台内核级实现。本包保留
// Bootstrap/Dialer 合同和可注入 Driver 适配点；默认实现明确返回 unsupported，
// 避免核心包绑定过时协议或不稳定 native 依赖。
package udt
