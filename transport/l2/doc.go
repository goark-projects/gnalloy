// Package l2 提供数据链路层自定义协议的 transport 抽象。
//
// Linux native driver 使用 AF_PACKET；macOS/BSD 默认 driver 使用 BPF；Windows
// 默认 driver 使用 Npcap。BPF/Npcap 依赖平台设备、权限和外部运行时库，核心包
// 通过 Driver 接口承接实现，并把平台细节隔离在 transport/l2/internal/nativeframe。
package l2
