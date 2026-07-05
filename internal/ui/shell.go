// Package ui 封装 WorkTide 的 TUI 主壳：主布局、侧边导航、全局快捷键、
// 帮助面板、主题与终端尺寸自适应。
//
// 本包是唯一依赖 bubbletea 的地方；业务与工具层通过 tools 包的稳定抽象交互，
// 从而满足 spec app-shell「UI 框架隔离」要求。
package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/tools"
	"github.com/sunzhenkai/worktide/internal/ui/theme"
)

// Focus 表示当前输入焦点区域。
type Focus int

const (
	// FocusNav 焦点在侧边导航。
	FocusNav Focus = iota
	// FocusContent 焦点在主内容区。
	FocusContent
)

// 窄屏阈值：宽度低于此值进入可滚动降级模式（不崩溃）。
const narrowWidth = 50

// Shell 是 WorkTide 的顶层 tea.Model，承载布局与全局交互。
type Shell struct {
	registry *tools.Registry
	renderer *Renderer
	keymap   Keymap
	cfg      *config.Config

	width, height int
	narrow        bool

	focus      Focus
	cursor     int // 导航列表选中项
	navItems   []tools.Meta
	showHelp   bool
	showSet    bool
	statusLine string

	// 帮助文案（按当前 keymap 生成）。
	helpText string
}

// Options 是构造 Shell 所需的依赖。
type Options struct {
	Registry *tools.Registry
	Config   *config.Config
	Theme    *theme.Theme
}

// NewShell 创建主壳模型，并按已启用工具初始化导航列表。
func NewShell(opts Options) *Shell {
	t := opts.Theme
	if t == nil {
		t = theme.Default()
	}
	km := keymapFromConfig(opts.Config)
	s := &Shell{
		registry: opts.Registry,
		renderer: NewRenderer(t),
		keymap:   km,
		cfg:      opts.Config,
		focus:    FocusContent,
	}
	s.rebuildNav()
	s.rebuildHelp()
	return s
}

// keymapFromConfig 从配置构建 Keymap。
func keymapFromConfig(cfg *config.Config) Keymap {
	km := Keymap{Quit: "q", FocusNav: "tab", Help: "?", Settings: "s"}
	if cfg != nil {
		if cfg.Keymap.Quit != "" {
			km.Quit = cfg.Keymap.Quit
		}
		if cfg.Keymap.FocusNav != "" {
			km.FocusNav = cfg.Keymap.FocusNav
		}
		if cfg.Keymap.Help != "" {
			km.Help = cfg.Keymap.Help
		}
		if cfg.Keymap.Settings != "" {
			km.Settings = cfg.Keymap.Settings
		}
	}
	return km
}

// rebuildNav 根据已启用工具重建导航项。
func (s *Shell) rebuildNav() {
	s.navItems = s.registry.Enabled()
	if s.cursor >= len(s.navItems) {
		s.cursor = 0
	}
}

// rebuildHelp 按当前 keymap 重建帮助文案。
func (s *Shell) rebuildHelp() {
	s.helpText = strings.Join([]string{
		fmt.Sprintf("%s  退出应用", displayBind(s.keymap.Quit)),
		fmt.Sprintf("%s  切换焦点（导航 <-> 内容）", displayBind(s.keymap.FocusNav)),
		fmt.Sprintf("%s  打开/关闭帮助", displayBind(s.keymap.Help)),
		fmt.Sprintf("%s  打开设置", displayBind(s.keymap.Settings)),
		"↑/↓   上下选择工具（导航焦点时）",
		"回车  激活选中工具",
		"1-9   快速切换到对应工具",
	}, "\n")
}

// displayBind 把绑定字符串规范化展示。
func displayBind(b string) string {
	switch strings.ToLower(b) {
	case "tab":
		return "Tab"
	case "esc":
		return "Esc"
	case "enter":
		return "Enter"
	}
	return strings.ToUpper(b)
}

// SetTheme 热切换主题。
func (s *Shell) SetTheme(t *theme.Theme) {
	s.renderer.SetTheme(t)
}

// ApplyConfig 应用新的配置快照（热加载回调使用）。
// 主题与快捷键即时生效；工具启用列表变更需重启（仅提示）。
func (s *Shell) ApplyConfig(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	old := s.cfg
	s.cfg = cfg
	// 主题即时生效。
	s.SetTheme(theme.Lookup(cfg.Theme.Name))
	// 快捷键即时生效。
	s.keymap = keymapFromConfig(cfg)
	s.rebuildHelp()
	// 工具启用列表变更：需要重启才生效，返回 true 提示调用方。
	toolsChanged := old == nil ||
		!stringSliceEqual(old.Tools.Enabled, cfg.Tools.Enabled)
	return toolsChanged
}

// stringSliceEqual 判断两个字符串切片是否相等。
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- tea.Model 实现 ----

// Init 启动时默认激活第一个工具。
func (s *Shell) Init() tea.Cmd {
	return s.activateCurrent(context.Background())
}

// activateCurrent 返回激活当前选中工具的命令。
func (s *Shell) activateCurrent(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if len(s.navItems) == 0 {
			return nil
		}
		id := s.navItems[s.cursor].ID
		_ = s.registry.Activate(ctx, id)
		return contentRenderedMsg{}
	}
}

// contentRenderedMsg 工具激活/重绘后触发刷新。
type contentRenderedMsg struct{}

// Update 处理消息与按键。
func (s *Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		s.narrow = msg.Width < narrowWidth
		// 通知激活工具尺寸变化（经路由）。
		s.registry.DispatchKey(tools.Key{Type: tools.KeyResize})
		return s, nil

	case tea.KeyMsg:
		// 全局快捷键优先（无论焦点）。
		if s.keymap.MatchQuit(msg) {
			return s, tea.Quit
		}
		if s.keymap.MatchHelp(msg) {
			s.showHelp = !s.showHelp
			return s, nil
		}
		if s.keymap.MatchSettings(msg) {
			s.showSet = !s.showSet
			return s, nil
		}
		if s.keymap.MatchFocusNav(msg) {
			if s.focus == FocusNav {
				s.focus = FocusContent
			} else {
				s.focus = FocusNav
			}
			return s, nil
		}

		// 焦点在导航：处理导航键。
		if s.focus == FocusNav {
			if cmd, handled := s.handleNavKey(msg); handled {
				return s, cmd
			}
		}

		// 数字快捷键：切换到对应工具（任意焦点下生效）。
		if cmd, handled := s.handleDigitKey(msg); handled {
			return s, cmd
		}

		// 否则：路由到激活工具。
		result := s.registry.DispatchKey(AdaptKey(msg))
		switch result.Effect {
		case tools.EffectQuit:
			return s, tea.Quit
		}
		return s, nil
	}
	return s, nil
}

// handleNavKey 处理导航焦点下的按键，返回 (命令, 是否已处理)。
func (s *Shell) handleNavKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		if s.cursor > 0 {
			s.cursor--
		}
		return nil, true
	case tea.KeyDown:
		if s.cursor < len(s.navItems)-1 {
			s.cursor++
		}
		return nil, true
	case tea.KeyEnter:
		s.focus = FocusContent
		return s.activateCurrent(context.Background()), true
	}
	return nil, false
}

// handleDigitKey 处理 1-9 快速切换工具。
func (s *Shell) handleDigitKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return nil, false
	}
	r := msg.Runes[0]
	if r < '1' || r > '9' {
		return nil, false
	}
	idx := int(r - '1')
	if idx >= len(s.navItems) {
		return nil, false
	}
	s.cursor = idx
	return s.activateCurrent(context.Background()), true
}

// View 渲染整个界面。
func (s *Shell) View() string {
	if s.width == 0 {
		return "正在启动 WorkTide..."
	}
	if s.narrow {
		return s.viewNarrow()
	}

	navWidth := 24
	contentWidth := s.width - navWidth - 2 // 留出分隔
	if contentWidth < 10 {
		contentWidth = 10
	}
	contentHeight := s.height
	if s.showHelp || s.showSet {
		contentHeight -= 8
		if contentHeight < 3 {
			contentHeight = 3
		}
	}

	nav := s.renderNav(navWidth, s.height)
	content := s.renderContent(contentWidth, contentHeight)

	layout := lipgloss.JoinHorizontal(lipgloss.Top,
		nav,
		s.renderer.Theme().Style(theme.KeyBorder).Render("│"),
		content,
	)

	if s.showHelp {
		layout = lipgloss.JoinVertical(lipgloss.Left,
			layout,
			s.renderPanel("帮助", s.helpText, s.width),
		)
	}
	if s.showSet {
		layout = lipgloss.JoinVertical(lipgloss.Left,
			layout,
			s.renderPanel("设置", s.renderSettings(), s.width),
		)
	}
	return layout
}

// viewNarrow 窄屏降级渲染：仅显示内容区，可滚动语义。
func (s *Shell) viewNarrow() string {
	content := s.renderContent(s.width, s.height-2)
	status := s.renderer.Theme().Style(theme.KeyMuted).
		Render(fmt.Sprintf("（窄屏模式 · %s 切换焦点）", displayBind(s.keymap.FocusNav)))
	return lipgloss.JoinVertical(lipgloss.Left, content, status)
}

// renderNav 渲染侧边导航。
func (s *Shell) renderNav(width, height int) string {
	title := s.renderer.Theme().Style(theme.KeyTitle).Render("WorkTide")
	var items []string
	for i, m := range s.navItems {
		label := fmt.Sprintf("%d %s %s", i+1, m.Icon, m.Name)
		label = strings.TrimSpace(label)
		if i == s.cursor && s.focus == FocusNav {
			items = append(items, s.renderer.Theme().Style(theme.KeyNavActive).Render(label))
		} else if m.ID == s.registry.ActiveID() {
			items = append(items, s.renderer.Theme().Style(theme.KeyAccent).Render("› "+label))
		} else {
			items = append(items, s.renderer.Theme().Style(theme.KeyNavItem).Render("  "+label))
		}
	}
	if len(items) == 0 {
		items = []string{s.renderer.Theme().Style(theme.KeyMuted).Render("（无可用工具）")}
	}
	body := lipgloss.JoinVertical(lipgloss.Left, items...)
	style := s.renderer.Theme().Style(theme.KeyBorder).
		Width(width).
		Height(height).
		Padding(0, 1)
	return style.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body))
}

// renderContent 渲染主内容区（激活工具视图）。
func (s *Shell) renderContent(width, height int) string {
	bounds := tools.Bounds{Width: width - 2, Height: height - 2}
	view := s.registry.ViewActive(bounds, s.renderer)
	if strings.TrimSpace(view) == "" {
		view = s.renderer.Theme().Style(theme.KeyMuted).
			Render("选择左侧工具开始工作。")
	}
	style := s.renderer.Theme().Style(theme.KeyContent).
		Width(width).
		Height(height).
		Padding(0, 1)
	return style.Render(view)
}

// renderPanel 渲染底部浮层面板（帮助/设置）。
func (s *Shell) renderPanel(title, body string, width int) string {
	border := s.renderer.Theme().Style(theme.KeyBorder).
		Width(width).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		PaddingTop(0)
	t := s.renderer.Theme().Style(theme.KeyTitle).Render(title)
	return border.Render(lipgloss.JoinVertical(lipgloss.Left, t, body))
}

// renderSettings 渲染设置面板内容。
func (s *Shell) renderSettings() string {
	if s.cfg == nil {
		return "（无配置）"
	}
	themeName := s.cfg.Theme.Name
	enabled := strings.Join(s.cfg.Tools.Enabled, ", ")
	if enabled == "" {
		enabled = "（空，使用默认）"
	}
	backend := "关闭"
	if s.cfg.Backend.Enabled {
		backend = "开启"
	}
	return strings.Join([]string{
		fmt.Sprintf("主题:     %s", themeName),
		fmt.Sprintf("启用工具: %s", enabled),
		fmt.Sprintf("本地后端: %s", backend),
		s.renderer.Theme().Style(theme.KeyMuted).
			Render("工具启用列表变更需重启应用生效。"),
	}, "\n")
}

// （调试钩子预留：如需将来接入 charmbracelet/log 可在此扩展。）
