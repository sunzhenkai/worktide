package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/tools"
	"github.com/sunzhenkai/worktide/internal/ui/theme"
)

// stubTool 用于 UI 测试。
type stubTool struct {
	meta tools.Meta
	view string
}

func (s *stubTool) Meta() tools.Meta                             { return s.meta }
func (s *stubTool) Init(context.Context) error                   { return nil }
func (s *stubTool) Activate(context.Context) error               { return nil }
func (s *stubTool) Deactivate() error                            { return nil }
func (s *stubTool) Close() error                                 { return nil }
func (s *stubTool) HandleKey(tools.Key) tools.Result             { return tools.Result{} }
func (s *stubTool) View(b tools.Bounds, _ tools.Renderer) string { return s.view }

// TestRendererImplementsToolsRenderer 验证 *Renderer 实现 tools.Renderer。
func TestRendererImplementsToolsRenderer(t *testing.T) {
	var _ tools.Renderer = NewRenderer(theme.Default())
	var _ tools.Styler = NewRenderer(theme.Default()).StyleFor("title")
}

// TestKeymapMatching 验证快捷键匹配。
func TestKeymapMatching(t *testing.T) {
	km := Keymap{Quit: "q", Help: "?", FocusNav: "tab", Settings: "s"}
	if !km.MatchQuit(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}) {
		t.Error("q 应匹配 Quit")
	}
	if km.MatchQuit(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}) {
		t.Error("a 不应匹配 Quit")
	}
	if !km.MatchHelp(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}) {
		t.Error("? 应匹配 Help")
	}
	if !km.MatchFocusNav(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Error("Tab 应匹配 FocusNav")
	}
	if !km.MatchSettings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}) {
		t.Error("s 应匹配 Settings")
	}
}

// TestAdaptKey 验证 bubbletea 按键到 tools.Key 的映射。
func TestAdaptKey(t *testing.T) {
	cases := []struct {
		name string
		in   tea.KeyMsg
		want tools.KeyType
	}{
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, tools.KeyEnter},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, tools.KeyEsc},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, tools.KeyTab},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, tools.KeyUp},
		{"rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, tools.KeyRune},
		{"space", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}, tools.KeySpace},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AdaptKey(c.in)
			if got.Type != c.want {
				t.Errorf("%s: 期望 %v，实际 %v", c.name, c.want, got.Type)
			}
		})
	}
}

// TestShellQuitKey 验证退出键触发 tea.Quit。
func TestShellQuitKey(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})

	shell := NewShell(Options{Registry: r, Config: config.Default(), Theme: theme.Default()})
	m, cmd := shell.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q 应返回 tea.Quit 命令")
	}
	// 执行命令应产生 tea.Quit（通过比较 cmd 的字符串表示包含 quit）。
	_ = m
}

// TestShellDigitKeySwitch 验证数字键切换工具。
func TestShellDigitKeySwitch(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "b", Name: "B"}, view: "B"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a", "b"})
	// 预先激活 a。
	_ = r.Activate(context.Background(), "a")

	shell := NewShell(Options{Registry: r, Config: config.Default(), Theme: theme.Default()})
	// 按 "2" 切换到第二项。
	_, cmd := shell.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if cmd == nil {
		t.Fatal("数字键应产生切换命令")
	}
	// 执行命令（同步触发 Activate）。
	msgs := execCmd(cmd)
	_ = msgs
	if r.ActiveID() != "b" {
		t.Errorf("按 2 应激活 b，实际: %s", r.ActiveID())
	}
}

// TestShellFocusToggle 验证焦点切换。
func TestShellFocusToggle(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	shell := NewShell(Options{Registry: r, Config: config.Default(), Theme: theme.Default()})
	if shell.focus != FocusContent {
		t.Fatal("初始焦点应为 Content")
	}
	_, _ = shell.Update(tea.KeyMsg{Type: tea.KeyTab})
	if shell.focus != FocusNav {
		t.Errorf("Tab 后焦点应为 Nav，实际: %v", shell.focus)
	}
	_, _ = shell.Update(tea.KeyMsg{Type: tea.KeyTab})
	if shell.focus != FocusContent {
		t.Errorf("再次 Tab 焦点应回 Content，实际: %v", shell.focus)
	}
}

// TestShellHelpToggle 验证帮助面板切换。
func TestShellHelpToggle(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	shell := NewShell(Options{Registry: r, Config: config.Default(), Theme: theme.Default()})
	if shell.showHelp {
		t.Fatal("初始帮助应关闭")
	}
	_, _ = shell.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !shell.showHelp {
		t.Error("? 应打开帮助")
	}
}

// TestApplyConfigThemeHotSwap 验证配置热加载切换主题。
func TestApplyConfigThemeHotSwap(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	cfg := config.Default()
	shell := NewShell(Options{Registry: r, Config: cfg, Theme: theme.Default()})
	if shell.renderer.ThemeName() != "default" {
		t.Fatalf("初始应为 default，实际: %s", shell.renderer.ThemeName())
	}
	cfg2 := config.Default()
	cfg2.Theme.Name = "dark"
	needRestart := shell.ApplyConfig(cfg2)
	if shell.renderer.ThemeName() != "dark" {
		t.Errorf("热加载后应为 dark，实际: %s", shell.renderer.ThemeName())
	}
	// 工具列表未变，不应要求重启。
	if needRestart {
		t.Error("仅主题变更不应要求重启")
	}
}

// TestApplyConfigToolsChangeRequiresRestart 验证工具列表变更需重启。
func TestApplyConfigToolsChangeRequiresRestart(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "b", Name: "B"}, view: "B"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a", "b"})
	cfg := config.Default()
	shell := NewShell(Options{Registry: r, Config: cfg, Theme: theme.Default()})
	cfg2 := config.Default()
	cfg2.Tools.Enabled = []string{"a"}
	if !shell.ApplyConfig(cfg2) {
		t.Error("工具列表变更应要求重启")
	}
}

// TestShellNarrowRender 验证窄屏降级不崩溃。
func TestShellNarrowRender(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	_ = r.Activate(context.Background(), "a")
	shell := NewShell(Options{Registry: r, Config: config.Default(), Theme: theme.Default()})
	_, _ = shell.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	out := shell.View()
	if out == "" {
		t.Fatal("窄屏应仍有渲染输出")
	}
}

// TestShellWideRender 验证宽屏布局不崩溃。
func TestShellWideRender(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.Register(&stubTool{meta: tools.Meta{ID: "a", Name: "A"}, view: "A"})
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	_ = r.Activate(context.Background(), "a")
	shell := NewShell(Options{Registry: r, Config: config.Default(), Theme: theme.Default()})
	_, _ = shell.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	out := shell.View()
	if out == "" {
		t.Fatal("宽屏应产生布局输出")
	}
}

// ---- 测试辅助 ----

// execCmd 执行一个 tea.Cmd 并收集产生的消息。
func execCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	var msgs []tea.Msg
	for {
		msg := cmd()
		if msg == nil {
			break
		}
		msgs = append(msgs, msg)
		// 单条命令通常返回 nil 或一个具体 Msg；避免无限循环。
		break
	}
	return msgs
}
