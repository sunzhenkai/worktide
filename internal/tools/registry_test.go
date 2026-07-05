package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeTool 是用于测试的最小 Tool 实现。
type fakeTool struct {
	meta        Meta
	initErr     error
	activateErr error
	events      []string // 记录生命周期与事件
	lastKey     Key
	viewOut     string
}

func (f *fakeTool) Meta() Meta { return f.meta }
func (f *fakeTool) Init(_ context.Context) error {
	f.events = append(f.events, "init")
	return f.initErr
}
func (f *fakeTool) Activate(_ context.Context) error {
	f.events = append(f.events, "activate")
	return f.activateErr
}
func (f *fakeTool) Deactivate() error { f.events = append(f.events, "deactivate"); return nil }
func (f *fakeTool) Close() error      { f.events = append(f.events, "close"); return nil }
func (f *fakeTool) HandleKey(k Key) Result {
	f.events = append(f.events, fmt.Sprintf("key:%d", k.Type))
	f.lastKey = k
	return Result{Effect: EffectRerender}
}
func (f *fakeTool) View(_ Bounds, _ Renderer) string { return f.viewOut }

func newFakeTool(id, name string) *fakeTool {
	return &fakeTool{meta: Meta{ID: id, Name: name, Version: "1.0.0"}, viewOut: "view-" + id}
}

// TestRegisterRejectsBadID 校验非 kebab-case ID 被拒。
func TestRegisterRejectsBadID(t *testing.T) {
	r := NewRegistry()
	for _, bad := range []string{"", "UPPER", "has space", "1num", "under_score"} {
		t.Run(bad, func(t *testing.T) {
			tool := &fakeTool{meta: Meta{ID: bad, Name: bad}}
			if err := r.Register(tool); err == nil {
				t.Errorf("ID %q 应被拒绝", bad)
			}
		})
	}
}

// TestRegisterAcceptsKebab 校验合法 ID 被接受。
func TestRegisterAcceptsKebab(t *testing.T) {
	r := NewRegistry()
	for _, id := range []string{"welcome", "sys-info", "tool2", "a"} {
		t.Run(id, func(t *testing.T) {
			if err := r.Register(&fakeTool{meta: Meta{ID: id, Name: id}}); err != nil {
				t.Errorf("ID %q 应被接受，错误: %v", id, err)
			}
		})
	}
}

// TestRegisterDuplicateRejected 校验重复 ID 被拒。
func TestRegisterDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newFakeTool("welcome", "欢迎")); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	err := r.Register(newFakeTool("welcome", "欢迎2"))
	if !errors.Is(err, ErrDuplicateID) {
		t.Errorf("重复注册应返回 ErrDuplicateID，实际: %v", err)
	}
}

// TestApplyEnabledFilters 校验启用列表过滤。
func TestApplyEnabledFilters(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	b := newFakeTool("b", "B")
	c := newFakeTool("c", "C")
	for _, t2 := range []Tool{a, b, c} {
		if err := r.Register(t2); err != nil {
			t.Fatalf("注册失败: %v", err)
		}
	}

	ok, err := r.ApplyEnabled(context.Background(), []string{"a", "c"})
	if err != nil {
		t.Fatalf("ApplyEnabled 失败: %v", err)
	}
	if len(ok) != 2 || ok[0] != "a" || ok[1] != "c" {
		t.Errorf("应仅启用 a、c，实际: %v", ok)
	}
	enabled := r.Enabled()
	if len(enabled) != 2 {
		t.Errorf("Enabled 长度应为 2，实际: %d", len(enabled))
	}
	// b 不应被初始化。
	if len(b.events) != 0 {
		t.Errorf("未启用的工具不应被初始化，b.events=%v", b.events)
	}
}

// TestApplyEnabledUnknownIgnored 校验未知 ID 被忽略（不报错）。
func TestApplyEnabledUnknownIgnored(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(newFakeTool("a", "A"))
	ok, err := r.ApplyEnabled(context.Background(), []string{"a", "ghost"})
	if err != nil {
		t.Fatalf("未知 ID 不应导致错误: %v", err)
	}
	if len(ok) != 1 || ok[0] != "a" {
		t.Errorf("应仅启用 a，实际: %v", ok)
	}
}

// TestApplyEnabledEmptyFallback 校验空列表回退到全部已注册工具。
func TestApplyEnabledEmptyFallback(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(newFakeTool("a", "A"))
	_ = r.Register(newFakeTool("b", "B"))
	ok, err := r.ApplyEnabled(context.Background(), nil)
	if err != nil {
		t.Fatalf("空列表不应报错: %v", err)
	}
	if len(ok) != 2 {
		t.Errorf("空列表应启用全部，实际: %v", ok)
	}
}

// TestApplyEnabledInitFailureSkipped 校验初始化失败的工具被跳过。
func TestApplyEnabledInitFailureSkipped(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	b := newFakeTool("b", "B")
	b.initErr = errors.New("boom")
	_ = r.Register(a)
	_ = r.Register(b)
	ok, err := r.ApplyEnabled(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if len(ok) != 1 || ok[0] != "a" {
		t.Errorf("应仅启用初始化成功的 a，实际: %v", ok)
	}
}

// TestActivateLifecycle 校验切换工具时先 Deactivate 旧再 Activate 新。
func TestActivateLifecycle(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	b := newFakeTool("b", "B")
	_ = r.Register(a)
	_ = r.Register(b)
	_, _ = r.ApplyEnabled(context.Background(), []string{"a", "b"})

	// 清空之前 Init 产生的事件，专注激活生命周期。
	a.events = a.events[:0]
	b.events = b.events[:0]

	if err := r.Activate(context.Background(), "a"); err != nil {
		t.Fatalf("激活 a 失败: %v", err)
	}
	if r.ActiveID() != "a" {
		t.Errorf("应激活 a，实际: %s", r.ActiveID())
	}
	if err := r.Activate(context.Background(), "b"); err != nil {
		t.Fatalf("激活 b 失败: %v", err)
	}
	if r.ActiveID() != "b" {
		t.Errorf("应激活 b，实际: %s", r.ActiveID())
	}
	// a 应被 deactivate，b 应被 activate。
	joined := strings.Join(a.events, ",") + "|" + strings.Join(b.events, ",")
	if !strings.Contains(joined, "deactivate") {
		t.Errorf("a 应被 deactivate，a.events=%v", a.events)
	}
	if !strings.Contains(strings.Join(b.events, ","), "activate") {
		t.Errorf("b 应被 activate，b.events=%v", b.events)
	}
}

// TestActivateSameToolIdempotent 校验重复激活同一工具幂等。
func TestActivateSameToolIdempotent(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	_ = r.Register(a)
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	if err := r.Activate(context.Background(), "a"); err != nil {
		t.Fatalf("激活 a 失败: %v", err)
	}
	a.events = a.events[:0]
	if err := r.Activate(context.Background(), "a"); err != nil {
		t.Fatalf("幂等激活失败: %v", err)
	}
	// 幂等激活不应触发额外的 deactivate（因为是同一工具）。
	for _, e := range a.events {
		if e == "deactivate" {
			t.Errorf("幂等激活同一工具不应 deactivate，events=%v", a.events)
		}
	}
}

// TestActivateUnknownRejected 校验激活未启用/未注册工具失败。
func TestActivateUnknownRejected(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(newFakeTool("a", "A"))
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	if err := r.Activate(context.Background(), "ghost"); !errors.Is(err, ErrUnknownTool) {
		t.Errorf("激活未注册工具应返回 ErrUnknownTool，实际: %v", err)
	}
}

// TestActivateNotInEnabledRejected 校验激活已注册但未启用的工具失败。
func TestActivateNotInEnabledRejected(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(newFakeTool("a", "A"))
	_ = r.Register(newFakeTool("b", "B"))
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"}) // 仅启用 a
	if err := r.Activate(context.Background(), "b"); !errors.Is(err, ErrUnknownTool) {
		t.Errorf("激活未启用工具应返回 ErrUnknownTool，实际: %v", err)
	}
}

// TestDispatchKeyRoutes 校验事件路由到激活工具。
func TestDispatchKeyRoutes(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	_ = r.Register(a)
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	_ = r.Activate(context.Background(), "a")

	res := r.DispatchKey(Key{Type: KeyEnter})
	if res.Effect != EffectRerender {
		t.Errorf("期望 EffectRerender，实际: %v", res.Effect)
	}
	if a.lastKey.Type != KeyEnter {
		t.Errorf("事件应路由到 a，lastKey=%v", a.lastKey)
	}
}

// TestDispatchKeyNoActive 校验无激活工具时返回零值。
func TestDispatchKeyNoActive(t *testing.T) {
	r := NewRegistry()
	res := r.DispatchKey(Key{Type: KeyEnter})
	if res.Effect != EffectNone {
		t.Errorf("无激活工具应返回 EffectNone，实际: %v", res.Effect)
	}
}

// TestViewActive 校验渲染激活工具。
func TestViewActive(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	a.viewOut = "hello-view"
	_ = r.Register(a)
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	_ = r.Activate(context.Background(), "a")
	got := r.ViewActive(Bounds{Width: 80, Height: 24}, NoopRenderer{})
	if got != "hello-view" {
		t.Errorf("ViewActive 渲染错误，实际: %q", got)
	}
}

// TestCloseAll 校验退出时全部工具被关闭。
func TestCloseAll(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	b := newFakeTool("b", "B")
	_ = r.Register(a)
	_ = r.Register(b)
	_, _ = r.ApplyEnabled(context.Background(), []string{"a", "b"})
	_ = r.CloseAll()
	if !contains(a.events, "close") {
		t.Errorf("a 应被 close，events=%v", a.events)
	}
	if !contains(b.events, "close") {
		t.Errorf("b 应被 close，events=%v", b.events)
	}
}

// TestDispatchKeyPanicIsolated 校验工具 panic 被隔离。
func TestDispatchKeyPanicIsolated(t *testing.T) {
	r := NewRegistry()
	panicTool := &panicTool{meta: Meta{ID: "panic", Name: "P"}}
	_ = r.Register(panicTool)
	_, _ = r.ApplyEnabled(context.Background(), []string{"panic"})
	_ = r.Activate(context.Background(), "panic")
	// 不应 panic，应返回隔离后的 rerender。
	res := r.DispatchKey(Key{Type: KeyEnter})
	if res.Effect != EffectRerender {
		t.Errorf("panic 后应返回 EffectRerender，实际: %v", res.Effect)
	}
}

// TestDeactivateActive 校验停用当前工具。
func TestDeactivateActive(t *testing.T) {
	r := NewRegistry()
	a := newFakeTool("a", "A")
	_ = r.Register(a)
	_, _ = r.ApplyEnabled(context.Background(), []string{"a"})
	_ = r.Activate(context.Background(), "a")
	a.events = a.events[:0]
	if err := r.DeactivateActive(); err != nil {
		t.Fatalf("DeactivateActive 失败: %v", err)
	}
	if r.ActiveID() != "" {
		t.Errorf("停用后应无激活工具，实际: %s", r.ActiveID())
	}
	if !contains(a.events, "deactivate") {
		t.Errorf("a 应被 deactivate，events=%v", a.events)
	}
}

// ---- 测试辅助 ----

type panicTool struct{ meta Meta }

func (p *panicTool) Meta() Meta                       { return p.meta }
func (p *panicTool) Init(_ context.Context) error     { return nil }
func (p *panicTool) Activate(_ context.Context) error { return nil }
func (p *panicTool) Deactivate() error                { return nil }
func (p *panicTool) Close() error                     { return nil }
func (p *panicTool) HandleKey(_ Key) Result           { panic("boom") }
func (p *panicTool) View(_ Bounds, _ Renderer) string { return "" }

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}
