// Package main is the entry point for the worktide CLI.
//
// 早期阶段仅提供占位 cobra root：现阶段没有任何子命令，
// 完整子命令树（svc 等）将在后续阶段实现。
package main

import (
	"github.com/spf13/cobra"
)

// newRootCmd 构建 cobra 根命令。
// 当前为占位实现：执行时直接进入 TUI 主界面（由 main 入口分发）。
func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worktide",
		Short: "WorkTide 个性化终端工作流中心",
		Long: "WorkTide 是一个 TUI 个人中心，把日常分散的命令行工具聚合到一个可插拔、\n" +
			"可个性化、可离线运行的工作台。",
		SilenceUsage:  true,
		SilenceErrors: true,
		// 不提供 Run：根命令无子命令时由 main 入口触发 TUI。
	}
}
