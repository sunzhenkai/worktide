package ui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sunzhenkai/worktide/internal/tools"
	"github.com/sunzhenkai/worktide/internal/ui/theme"
)

// Renderer 把 Theme 适配为工具可用的 tools.Renderer。
// 工具通过它获取 Styler，避免直接依赖 lipgloss。
type Renderer struct {
	theme *theme.Theme
}

// NewRenderer 基于给定主题创建渲染器。
func NewRenderer(t *theme.Theme) *Renderer {
	return &Renderer{theme: t}
}

// SetTheme 在运行时切换主题（用于热加载）。
func (r *Renderer) SetTheme(t *theme.Theme) {
	if t != nil {
		r.theme = t
	}
}

// Theme 返回当前主题（供 UI 自身使用）。
func (r *Renderer) Theme() *theme.Theme { return r.theme }

// StyleFor 实现 tools.Renderer。
func (r *Renderer) StyleFor(name string) tools.Styler {
	return styler{style: r.theme.Style(name)}
}

// ThemeName 实现 tools.Renderer。
func (r *Renderer) ThemeName() string { return r.theme.Name() }

// styler 把 lipgloss.Style 适配为 tools.Styler。
type styler struct{ style lipgloss.Style }

// Render 实现 tools.Styler。
func (s styler) Render(str string) string { return s.style.Render(str) }

// ---- 按键适配 ----

// AdaptKey 把 bubbletea 的 tea.KeyMsg 转为工具层稳定的 tools.Key。
// 这样工具实现仅依赖 tools.Key，不耦合 bubbletea。
func AdaptKey(msg tea.KeyMsg) tools.Key {
	k := tools.Key{Alt: msg.Alt}
	switch msg.Type {
	case tea.KeyRunes:
		k.Type = tools.KeyRune
		if len(msg.Runes) > 0 {
			k.Rune = msg.Runes[0]
		}
		if len(msg.Runes) == 1 && msg.Runes[0] == ' ' {
			k.Type = tools.KeySpace
		}
	case tea.KeyEsc:
		k.Type = tools.KeyEsc
	case tea.KeyEnter:
		k.Type = tools.KeyEnter
	case tea.KeyBackspace:
		k.Type = tools.KeyBackspace
	case tea.KeyTab:
		k.Type = tools.KeyTab
	case tea.KeyUp:
		k.Type = tools.KeyUp
	case tea.KeyDown:
		k.Type = tools.KeyDown
	case tea.KeyLeft:
		k.Type = tools.KeyLeft
	case tea.KeyRight:
		k.Type = tools.KeyRight
	default:
		k.Type = tools.KeyUnknown
	}
	return k
}

// Keymap 持有从配置解析出的全局快捷键绑定。
type Keymap struct {
	Quit     string
	FocusNav string
	Help     string
	Settings string
}

// matches 判断给定按键是否对应配置项 bind（bind 为 "q"/"?" 等单字符或 "tab"/"esc" 等名）。
func matches(msg tea.KeyMsg, bind string) bool {
	if bind == "" {
		return false
	}
	switch strings.ToLower(bind) {
	case "tab":
		return msg.Type == tea.KeyTab
	case "esc":
		return msg.Type == tea.KeyEsc
	case "enter":
		return msg.Type == tea.KeyEnter
	case "space":
		return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == ' '
	default:
		if len(bind) == 1 && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			return msg.Runes[0] == rune(bind[0])
		}
	}
	return false
}

// MatchQuit / MatchFocusNav / MatchHelp / MatchSettings 判断按键是否命中对应绑定。
func (k Keymap) MatchQuit(msg tea.KeyMsg) bool     { return matches(msg, k.Quit) }
func (k Keymap) MatchFocusNav(msg tea.KeyMsg) bool { return matches(msg, k.FocusNav) }
func (k Keymap) MatchHelp(msg tea.KeyMsg) bool     { return matches(msg, k.Help) }
func (k Keymap) MatchSettings(msg tea.KeyMsg) bool { return matches(msg, k.Settings) }
