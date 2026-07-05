package ui

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/tools"
	"github.com/sunzhenkai/worktide/internal/ui/theme"
)

// Program 封装 bubbletea 程序的创建与运行，屏蔽具体框架类型。
// 调用方（app 层）仅通过 Run 启动，不直接接触 tea.Program。
type Program struct {
	shell   *Shell
	program *tea.Program
}

// NewProgram 基于注册中心与配置创建一个 TUI 程序。
// 主题按配置解析，未配置时使用默认主题。
func NewProgram(registry *tools.Registry, cfg *config.Config, output io.Writer) *Program {
	t := theme.Default()
	if cfg != nil {
		t = theme.Lookup(cfg.Theme.Name)
	}
	shell := NewShell(Options{Registry: registry, Config: cfg, Theme: t})
	opts := []tea.ProgramOption{
		tea.WithAltScreen(),
	}
	if output != nil {
		opts = append(opts, tea.WithoutSignalHandler())
	}
	p := tea.NewProgram(shell, opts...)
	return &Program{shell: shell, program: p}
}

// Run 启动并阻塞运行 TUI 程序，直到退出。
// 上下文取消会向程序发送 Kill 以快速结束。
func (p *Program) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		p.program.Kill()
	}()

	if _, err := p.program.Run(); err != nil {
		return fmt.Errorf("TUI 程序运行失败: %w", err)
	}
	return nil
}

// Shell 返回内部 Shell 引用（用于热加载配置/主题）。
func (p *Program) Shell() *Shell { return p.shell }

// ApplyConfig 应用新配置快照，返回是否需要重启（工具启用列表变更）。
func (p *Program) ApplyConfig(cfg *config.Config) bool {
	return p.shell.ApplyConfig(cfg)
}
