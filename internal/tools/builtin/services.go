package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/service"
	"github.com/sunzhenkai/worktide/internal/tools"
)

// Services 是服务管理工具。
// 形态：双面板（左：服务列表 / 右：选中服务的日志预览 + 操作提示）。
type Services struct {
	meta  tools.Meta
	mgr   *service.Manager
	paths config.Paths
	mu    sync.Mutex
	// list 是当前已渲染的 entry 列表。
	list []*service.Entry
	// cursor 是当前选中的索引。
	cursor int
	// previewLines 是当前选中服务的日志预览。
	previewLines []string
	// following 表示是否正在 follow 日志。
	following bool
	// showingHelp 表示是否显示帮助面板。
	showingHelp bool
	// lastError 是最近一次操作的错误（用于显示提示）。
	lastError string
	// lastMessage 是最近一次成功的提示。
	lastMessage string
	// pollCancel 停止状态轮询。
	pollCancel context.CancelFunc
}

// NewServices 创建一个 Services Tool。
// 若 mgr 为 nil，工具仍能渲染（仅显示空状态）。
func NewServices(mgr *service.Manager, paths config.Paths) *Services {
	return &Services{
		meta: tools.Meta{
			ID:          "services",
			Name:        "服务",
			Description: "管理本地长驻服务：列表、状态、日志、启停",
			Icon:        "⚙",
			Version:     "0.1.0",
		},
		mgr:   mgr,
		paths: paths,
	}
}

// Meta 返回元信息。
func (s *Services) Meta() tools.Meta { return s.meta }

// Init 初始化（启动状态轮询）。
func (s *Services) Init(ctx context.Context) error {
	if s.mgr == nil {
		return nil
	}
	pollCtx, cancel := context.WithCancel(ctx)
	s.pollCancel = cancel
	go s.pollLoop(pollCtx)
	return nil
}

// pollLoop 周期性刷新 list 与 preview。
func (s *Services) pollLoop(ctx context.Context) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshList()
			s.refreshPreview()
		}
	}
}

func (s *Services) refreshList() {
	if s.mgr == nil {
		return
	}
	list, err := s.mgr.List()
	if err != nil {
		slog.Warn("List 失败", "error", err)
		return
	}
	s.mu.Lock()
	s.list = list
	if s.cursor >= len(list) {
		s.cursor = max(0, len(list)-1)
	}
	s.mu.Unlock()
}

func (s *Services) refreshPreview() {
	if s.mgr == nil {
		return
	}
	s.mu.Lock()
	if s.cursor < 0 || s.cursor >= len(s.list) {
		s.mu.Unlock()
		return
	}
	name := s.list[s.cursor].Name
	logPath := s.mgr.LogPath(name)
	s.mu.Unlock()
	lines, err := service.ReadTailLines(logPath, 30)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.previewLines = lines
	s.mu.Unlock()
}

// Activate 激活工具。
func (s *Services) Activate(_ context.Context) error {
	s.refreshList()
	s.refreshPreview()
	return nil
}

// Deactivate 停用。
func (s *Services) Deactivate() error { return nil }

// Close 释放资源。
func (s *Services) Close() error {
	if s.pollCancel != nil {
		s.pollCancel()
	}
	return nil
}

// HandleKey 处理按键。
func (s *Services) HandleKey(key tools.Key) tools.Result {
	if s.mgr == nil {
		return tools.Result{Effect: tools.EffectRerender}
	}
	s.mu.Lock()
	showingHelp := s.showingHelp
	s.mu.Unlock()
	if showingHelp {
		if key.Type == tools.KeyRune && (key.Rune == '?' || key.Rune == 'q' || key.Rune == 'h') {
			s.mu.Lock()
			s.showingHelp = false
			s.mu.Unlock()
		}
		return tools.Result{Effect: tools.EffectRerender}
	}
	switch {
	case key.Type == tools.KeyRune && key.Rune == '?':
		s.mu.Lock()
		s.showingHelp = true
		s.mu.Unlock()
	case key.Type == tools.KeyUp:
		s.moveCursor(-1)
	case key.Type == tools.KeyDown:
		s.moveCursor(+1)
	case key.Type == tools.KeyRune && key.Rune == 'j':
		s.moveCursor(+1)
	case key.Type == tools.KeyRune && key.Rune == 'k':
		s.moveCursor(-1)
	case key.Type == tools.KeyRune && key.Rune == 'r':
		s.doRestart()
	case key.Type == tools.KeyRune && key.Rune == 'k' && false: // 不可达分支：上方已匹配 k
	case key.Type == tools.KeyRune && key.Rune == 'K':
		s.doKill(true)
	case key.Type == tools.KeyRune && key.Rune == 's':
		s.doStart()
	case key.Type == tools.KeyRune && key.Rune == 'l':
		s.toggleFollow()
	case key.Type == tools.KeyRune && key.Rune == 'x':
		s.doClean()
	}
	return tools.Result{Effect: tools.EffectRerender}
}

// moveCursor 移动光标。
func (s *Services) moveCursor(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.list) == 0 {
		return
	}
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.list) {
		s.cursor = len(s.list) - 1
	}
}

func (s *Services) selectedName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursor < 0 || s.cursor >= len(s.list) {
		return ""
	}
	return s.list[s.cursor].Name
}

func (s *Services) doRestart() {
	name := s.selectedName()
	if name == "" {
		return
	}
	_, err := s.mgr.Restart(name)
	s.recordResult(err, fmt.Sprintf("restarted %q", name))
}

func (s *Services) doKill(force bool) {
	name := s.selectedName()
	if name == "" {
		return
	}
	err := s.mgr.Kill(name, force)
	s.recordResult(err, fmt.Sprintf("killed %q", name))
}

func (s *Services) doStart() {
	name := s.selectedName()
	if name == "" {
		return
	}
	s.mu.Lock()
	decl, hasDecl := s.mgr.Decls()[name]
	s.mu.Unlock()
	if !hasDecl {
		s.setError(fmt.Sprintf("no declaration for %q", name))
		return
	}
	_, err := s.mgr.Run(service.RunOptions{
		Name:    name,
		Cwd:     decl.Cwd,
		Command: decl.Command,
		Env:     decl.Env,
	})
	s.recordResult(err, fmt.Sprintf("started %q", name))
}

func (s *Services) toggleFollow() {
	s.mu.Lock()
	s.following = !s.following
	s.mu.Unlock()
}

func (s *Services) doClean() {
	_, err := s.mgr.Clean(service.CleanOptions{})
	s.recordResult(err, "cleaned")
}

func (s *Services) setError(s2 string) {
	s.mu.Lock()
	s.lastError = s2
	s.lastMessage = ""
	s.mu.Unlock()
}

func (s *Services) recordResult(err error, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.lastError = err.Error()
		s.lastMessage = ""
	} else {
		s.lastError = ""
		s.lastMessage = msg
	}
}

// View 渲染双面板视图。
func (s *Services) View(b tools.Bounds, r tools.Renderer) string {
	if s.mgr == nil {
		return r.StyleFor("muted").Render("服务管理器未初始化（无 bbolt 数据库）")
	}
	s.mu.Lock()
	list := s.list
	cursor := s.cursor
	preview := s.previewLines
	following := s.following
	showingHelp := s.showingHelp
	lastErr := s.lastError
	lastMsg := s.lastMessage
	s.mu.Unlock()

	if showingHelp {
		return renderHelp(r)
	}

	width := b.Width
	height := b.Height
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	// 左：list（占 30% 宽度）
	leftW := width * 30 / 100
	if leftW < 20 {
		leftW = 20
	}
	rightW := width - leftW - 1

	left := renderList(r, list, cursor, leftW, height-2)
	right := renderPreview(r, nameOf(list, cursor), preview, following, rightW, height-2)

	body := padRow(left, leftW) + "│" + padRow(right, rightW)

	footer := renderFooter(r, lastErr, lastMsg, width, height)
	return body + "\n" + footer
}

func nameOf(list []*service.Entry, cursor int) string {
	if cursor < 0 || cursor >= len(list) {
		return ""
	}
	return list[cursor].Name
}

func renderList(r tools.Renderer, list []*service.Entry, cursor, width, height int) string {
	title := r.StyleFor("title").Render("服务")
	var rows []string
	rows = append(rows, title)
	rows = append(rows, strings.Repeat("─", width))
	if len(list) == 0 {
		rows = append(rows, r.StyleFor("muted").Render("(no services)"))
	} else {
		for i, e := range list {
			indicator := statusIndicator(e.Status)
			name := fmt.Sprintf("%-12s", truncate(e.Name, 12))
			row := fmt.Sprintf("%s %s", indicator, name)
			if i == cursor {
				row = r.StyleFor("accent").Render("▶ " + row)
			} else {
				row = "  " + row
			}
			rows = append(rows, row)
		}
	}
	// 截断到 height 行
	if len(rows) > height {
		rows = rows[:height]
	}
	return strings.Join(rows, "\n")
}

func renderPreview(r tools.Renderer, name string, lines []string, following bool, width, height int) string {
	title := r.StyleFor("title").Render(fmt.Sprintf("日志：%s", name))
	var rows []string
	rows = append(rows, title)
	rows = append(rows, strings.Repeat("─", width))
	if len(lines) == 0 {
		rows = append(rows, r.StyleFor("muted").Render("(empty)"))
	} else {
		for _, l := range lines {
			rows = append(rows, truncate(l, width))
		}
	}
	if following {
		rows = append(rows, r.StyleFor("muted").Render("[following]"))
	}
	if len(rows) > height {
		// 仅保留末尾 height 行。
		rows = rows[len(rows)-height:]
	}
	return strings.Join(rows, "\n")
}

func renderHelp(r tools.Renderer) string {
	help := []string{
		"服务管理 - 按键说明",
		"─────────────────",
		"  ↑/k   上移",
		"  ↓/j   下移",
		"  r     restart 选中服务",
		"  K     kill -9 选中服务（大写 K）",
		"  s     start 选中服务（按声明启动）",
		"  l     切换日志跟随",
		"  x     clean（清理 exited/stale）",
		"  ?     显示/隐藏本帮助",
		"  q     退出应用（在主壳处理）",
		"",
		"按 ? 返回。",
	}
	return r.StyleFor("muted").Render(strings.Join(help, "\n"))
}

func renderFooter(r tools.Renderer, lastErr, lastMsg string, width, height int) string {
	if lastErr != "" {
		return r.StyleFor("error").Render("err: " + lastErr)
	}
	if lastMsg != "" {
		return r.StyleFor("muted").Render(lastMsg)
	}
	return r.StyleFor("muted").Render("[r] restart  [K] kill  [s] start  [l] follow  [x] clean  [?] help")
}

func statusIndicator(s service.Status) string {
	switch s {
	case service.StatusRunning:
		return "●"
	case service.StatusExited:
		return "○"
	default:
		return "×"
	}
}

func padRow(s string, width int) string {
	// 把 s 视为已有若干 \n 分隔的行；逐行填充。
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lv := len([]rune(l))
		if lv < width {
			lines[i] = l + strings.Repeat(" ", width-lv)
		} else if lv > width {
			// 截断
			rs := []rune(l)
			lines[i] = string(rs[:width])
		}
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= w {
		return s
	}
	return string(rs[:w-1]) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
