// Package service 提供 WorkTide 的本地服务管理能力。
//
// 职责：
//   - 管理服务声明（name -> cwd/command）与运行态（PID/PGID/LogPath）的分离；
//   - 启动 / 停止 / 重启 / 清理后台进程，支持 Setpgid 进程组隔离；
//   - 提供日志读取、tail/follow 与原子打开；
//   - 持久化运行态到 bbolt（bucket "services"），跨进程并发安全。
//
// 设计原则：
//   - 声明（YAML）与运行态（bbolt）严格分离；
//   - 不依赖具体 TUI/CLI 框架，CLI 与 TUI 共用同一 Manager；
//   - 进程组（Setpgid + kill -pgid）保证子进程可一并清理。
package service
