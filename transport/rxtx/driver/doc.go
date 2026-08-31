// Package driver 提供串口/RXTX 第三方实现到 gnalloy transport/rxtx 的适配器。
//
// 本包只保存边界类型和函数式后端，不引入具体串口库。真实实现应在独立
// module 中处理设备名、权限和平台驱动，并通过 rxtx.Config.Driver 注入。
package driver
