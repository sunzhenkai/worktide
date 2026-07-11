// Package main 的 CLI 入口。
//
// Execute 是 cmd/worktide 的 cobra 入口。
// 无 cobra 子命令时（os.Args 长度 == 1），返回 false 表示应进入 TUI 模式。
package main

import (
	"github.com/spf13/cobra"
)

// Execute 解析并执行 cobra 命令树。
// 返回 true 表示已处理（cobra 子命令已执行或 --help）；false 表示应当进入 TUI。
func Execute(args []string) (bool, error) {
	root := newRootCmd()
	// 默认 Run：空 args 时执行（app.Run），由 caller 拦截。
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		// 这里不应被调用：main 在 os.Args 长度为 1 时跳过 cobra。
		return nil
	}
	// 把子命令挂上。
	root.AddCommand(newSvcCmd())
	root.SetArgs(args)
	root.SilenceUsage = true
	root.SilenceErrors = true
	if err := root.Execute(); err != nil {
		return true, err
	}
	// Execute 不会因无子命令而返回错误；具体判定由 main 入口在 Execute 之前决定。
	return true, nil
}
