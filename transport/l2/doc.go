// Package l2 提供数据链路层自定义协议的 transport 抽象。
//
// Linux native driver 使用 AF_PACKET；macOS/BSD 默认边界对应 BPF；Windows 默认
// 边界对应 Npcap。BPF/Npcap 依赖平台设备和外部运行时能力，核心包通过 Driver
// 接口承接实现，避免在跨平台核心中硬绑定 cgo 或第三方动态库。
package l2
