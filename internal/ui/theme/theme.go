// Package theme 定义 WorkTide 的主题与样式抽象。
//
// 它桥接 lipgloss（具体样式库）与 tools.Renderer/Styler（稳定抽象），
// 使工具实现无需直接依赖 lipgloss 即可获得一致的视觉风格。
package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme 是一套命名的视觉样式集合。
type Theme struct {
	name   string
	styles map[string]lipgloss.Style
}

// Name 返回主题名。
func (t *Theme) Name() string { return t.name }

// Style 返回指定语义键的 lipgloss 样式；未定义时返回中性默认样式。
func (t *Theme) Style(key string) lipgloss.Style {
	if s, ok := t.styles[key]; ok {
		return s
	}
	return lipgloss.NewStyle()
}

// 语义样式键常量，工具与 UI 共享。
const (
	KeyTitle     = "title"
	KeySubtitle  = "subtitle"
	KeyMuted     = "muted"
	KeyAccent    = "accent"
	KeyBorder    = "border"
	KeyNavActive = "nav_active"
	KeyNavItem   = "nav_item"
	KeyContent   = "content"
	KeyHelp      = "help"
	KeyError     = "error"
	KeySuccess   = "success"
)

// Default 返回默认主题（浅色友好、高对比）。
func Default() *Theme {
	return &Theme{
		name: "default",
		styles: map[string]lipgloss.Style{
			KeyTitle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")),
			KeySubtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
			KeyMuted:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
			KeyAccent:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("201")),
			KeyBorder:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
			KeyNavActive: lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("63")).
				Padding(0, 1),
			KeyNavItem: lipgloss.NewStyle().Padding(0, 1),
			KeyContent: lipgloss.NewStyle().Padding(0, 1),
			KeyHelp:    lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
			KeyError:   lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
			KeySuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("36")),
		},
	}
}

// Dark 返回深色主题。
func Dark() *Theme {
	return &Theme{
		name: "dark",
		styles: map[string]lipgloss.Style{
			KeyTitle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")),
			KeySubtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
			KeyMuted:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
			KeyAccent:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141")),
			KeyBorder:   lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
			KeyNavActive: lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("236")).
				Background(lipgloss.Color("141")).
				Padding(0, 1),
			KeyNavItem: lipgloss.NewStyle().Padding(0, 1),
			KeyContent: lipgloss.NewStyle().Padding(0, 1),
			KeyHelp:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
			KeyError:   lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Bold(true),
			KeySuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		},
	}
}

// Lookup 按名称返回主题，未知名回退到默认主题。
func Lookup(name string) *Theme {
	switch name {
	case "dark":
		return Dark()
	default:
		return Default()
	}
}

// Styles 访问器，便于在测试中检查。
func (t *Theme) Styles() map[string]lipgloss.Style {
	out := make(map[string]lipgloss.Style, len(t.styles))
	for k, v := range t.styles {
		out[k] = v.Copy()
	}
	return out
}
