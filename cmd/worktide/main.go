// Command worktide 是 WorkTide TUI 个人中心与 CLI 的统一入口。
//
// 入口分发：
//   - 无 cobra 子命令（os.Args 长度 == 1）→ app.Run（进入 TUI）
//   - 有 cobra 子命令 → cli.Execute（执行 CLI）
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sunzhenkai/worktide/internal/app"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		// 无参数：进入 TUI。
		os.Exit(int(app.Run(context.Background(), args)))
		return
	}
	// 有参数：交给 cobra 解析。
	handled, err := Execute(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worktide: %v\n", err)
		os.Exit(1)
	}
	if !handled {
		os.Exit(int(app.Run(context.Background(), args)))
	}
}
