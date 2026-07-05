// Command worktide 是 WorkTide TUI 个人中心的入口。
//
// 它仅负责将控制权交给 internal/app.Run，并把返回的退出码透传给操作系统。
package main

import (
	"context"
	"os"

	"github.com/sunzhenkai/worktide/internal/app"
)

func main() {
	os.Exit(int(app.Run(context.Background(), os.Args[1:])))
}
