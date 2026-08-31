// Package driver 提供 UDT 第三方实现到 gnalloy transport/udt 的适配器。
//
// 本包只保存边界类型和函数式后端，不引入具体 UDT 库。真实实现应在独立
// module 中打开 socket、维护协议状态，并通过 udt.Config.Driver 注入。
package driver
