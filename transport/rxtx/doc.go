// Package rxtx 定义串口/RXTX 风格传输的 gnalloy 扩展边界。
//
// 串口实现依赖平台驱动、权限和设备名约定，本包只提供 Go-native client
// transport 合同与可注入 Driver；默认不绑定任何系统库。
package rxtx
