// Package npcap 提供 Windows Npcap 二层驱动的扩展边界。
//
// 本包只定义 Driver、Backend 和 Config 契约，不直接加载 Packet.dll、wpcap.dll
// 或其他 Npcap 动态库。真实 Npcap 后端应由独立扩展包实现 Backend 后注入到
// transport/l2.Config.Driver。
package npcap
