package builtin

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/sunzhenkai/worktide/internal/tools"
)

// startedAt 记录进程启动时间，用于计算运行时长。
var startedAt = time.Now()

// SysInfo 是系统信息工具：展示 Go 版本、平台、运行时长、目录路径等。
type SysInfo struct {
	meta tools.Meta
	// paths 提供 WorkTide 关键目录路径用于展示。
	paths SysPaths
}

// SysPaths 是注入到 SysInfo 的目录信息（避免直接依赖 config 包）。
type SysPaths struct {
	ConfigDir string
	DataDir   string
	CacheDir  string
	LogDir    string
}

// NewSysInfo 创建系统信息工具。
func NewSysInfo() *SysInfo {
	return &SysInfo{
		meta: tools.Meta{
			ID:          "sysinfo",
			Name:        "系统信息",
			Description: "运行环境、Go 版本、目录路径一览",
			Icon:        "🛈",
			Version:     "0.1.0",
		},
	}
}

// SetPaths 注入目录路径（由 app 层在装配时调用）。
func (s *SysInfo) SetPaths(p SysPaths) { s.paths = p }

// Meta 返回元信息。
func (s *SysInfo) Meta() tools.Meta { return s.meta }

// Init 无需初始化资源。
func (s *SysInfo) Init(_ context.Context) error { return nil }

// Activate 无副作用。
func (s *SysInfo) Activate(_ context.Context) error { return nil }

// Deactivate 无副作用。
func (s *SysInfo) Deactivate() error { return nil }

// Close 无资源释放。
func (s *SysInfo) Close() error { return nil }

// HandleKey 系统信息页不处理特殊按键。
func (s *SysInfo) HandleKey(_ tools.Key) tools.Result { return tools.Result{} }

// View 渲染系统信息。
func (s *SysInfo) View(b tools.Bounds, r tools.Renderer) string {
	title := r.StyleFor("title").Render("系统信息")

	rows := [][2]string{
		{"Go 版本", runtime.Version()},
		{"平台", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
		{"CPU 核心数", fmt.Sprintf("%d", runtime.NumCPU())},
		{"运行时长", time.Since(startedAt).Truncate(time.Second).String()},
		{"配置目录", defaultIfEmpty(s.paths.ConfigDir, "（未注入）")},
		{"数据目录", defaultIfEmpty(s.paths.DataDir, "（未注入）")},
		{"缓存目录", defaultIfEmpty(s.paths.CacheDir, "（未注入）")},
		{"日志目录", defaultIfEmpty(s.paths.LogDir, "（未注入）")},
	}

	var b2 strings.Builder
	for _, row := range rows {
		label := r.StyleFor("muted").Render(fmt.Sprintf("%-10s", row[0]))
		fmt.Fprintf(&b2, "%s  %s\n", label, row[1])
	}

	body := strings.Join([]string{
		title,
		"",
		b2.String(),
	}, "\n")

	_ = b.Width
	return body
}

// defaultIfEmpty 返回 v，若为空则返回 fallback。
func defaultIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
