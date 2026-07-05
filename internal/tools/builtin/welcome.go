package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunzhenkai/worktide/internal/tools"
)

// Welcome 是欢迎页工具：展示项目简介、快捷键提示与当前启用工具列表。
type Welcome struct {
	meta tools.Meta
}

// NewWelcome 创建欢迎页工具。
func NewWelcome() *Welcome {
	return &Welcome{
		meta: tools.Meta{
			ID:          "welcome",
			Name:        "欢迎",
			Description: "项目简介、快捷键与已启用工具一览",
			Icon:        "🏠",
			Version:     "0.1.0",
		},
	}
}

// Meta 返回元信息。
func (w *Welcome) Meta() tools.Meta { return w.meta }

// Init 无需初始化资源。
func (w *Welcome) Init(_ context.Context) error { return nil }

// Activate 无副作用。
func (w *Welcome) Activate(_ context.Context) error { return nil }

// Deactivate 无副作用。
func (w *Welcome) Deactivate() error { return nil }

// Close 无资源释放。
func (w *Welcome) Close() error { return nil }

// HandleKey 欢迎页不处理特殊按键。
func (w *Welcome) HandleKey(_ tools.Key) tools.Result { return tools.Result{} }

// View 渲染欢迎页。
func (w *Welcome) View(b tools.Bounds, r tools.Renderer) string {
	title := r.StyleFor("title").Render("WorkTide")
	sub := r.StyleFor("subtitle").Render("你的个性化终端工作流中心")

	intro := []string{
		r.StyleFor("muted").Render("在这里，一切工具触手可及。"),
		"使用左侧导航或数字键 1-9 切换工具。",
		fmt.Sprintf("按 %s 打开帮助，%s 退出应用。",
			r.StyleFor("accent").Render("?"), r.StyleFor("accent").Render("q")),
	}

	body := strings.Join([]string{
		title,
		sub,
		"",
		strings.Join(intro, "\n"),
	}, "\n")

	width := b.Width - 2
	if width < 0 {
		width = 0
	}
	_ = width
	return body
}
