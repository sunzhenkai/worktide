package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/sunzhenkai/worktide/internal/tools"
)

// TestRegisterAll 校验 RegisterAll 登记了预期工具。
func TestRegisterAll(t *testing.T) {
	r := tools.NewRegistry()
	if err := RegisterAll(r, RegisterOptions{}); err != nil {
		t.Fatalf("RegisterAll 失败: %v", err)
	}
	ids := map[string]bool{}
	for _, m := range r.All() {
		ids[m.ID] = true
	}
	for _, want := range []string{"welcome", "sysinfo"} {
		if !ids[want] {
			t.Errorf("应注册 %s", want)
		}
	}
}

// TestRegisterAllDuplicate 校验重复注册返回错误。
func TestRegisterAllDuplicate(t *testing.T) {
	r := tools.NewRegistry()
	if err := RegisterAll(r, RegisterOptions{}); err != nil {
		t.Fatalf("首次 RegisterAll 失败: %v", err)
	}
	if err := RegisterAll(r, RegisterOptions{}); err == nil {
		t.Error("重复注册应返回错误")
	}
}

// TestWelcomeMeta 校验欢迎页元信息。
func TestWelcomeMeta(t *testing.T) {
	w := NewWelcome()
	m := w.Meta()
	if m.ID != "welcome" || m.Name == "" {
		t.Errorf("元信息异常: %+v", m)
	}
}

// TestWelcomeView 校验欢迎页渲染非空。
func TestWelcomeView(t *testing.T) {
	w := NewWelcome()
	out := w.View(tools.Bounds{Width: 80, Height: 24}, tools.NoopRenderer{})
	if !strings.Contains(out, "WorkTide") {
		t.Errorf("欢迎页应包含项目名，实际: %q", out)
	}
}

// TestSysInfoLifecycle 校验系统信息工具生命周期不报错。
func TestSysInfoLifecycle(t *testing.T) {
	s := NewSysInfo()
	s.SetPaths(SysPaths{ConfigDir: "/cfg", DataDir: "/data", CacheDir: "/cache", LogDir: "/logs"})
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	if err := s.Activate(ctx); err != nil {
		t.Fatalf("Activate 失败: %v", err)
	}
	if err := s.Deactivate(); err != nil {
		t.Fatalf("Deactivate 失败: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
}

// TestSysInfoView 校验系统信息页渲染。
func TestSysInfoView(t *testing.T) {
	s := NewSysInfo()
	s.SetPaths(SysPaths{ConfigDir: "/cfg/worktide", DataDir: "/data", CacheDir: "/cache", LogDir: "/logs"})
	out := s.View(tools.Bounds{Width: 80, Height: 24}, tools.NoopRenderer{})
	if !strings.Contains(out, "Go 版本") || !strings.Contains(out, "/cfg/worktide") {
		t.Errorf("系统信息页应包含 Go 版本与目录，实际: %q", out)
	}
}

// TestSysInfoViewWithoutPaths 校验未注入路径时降级展示。
func TestSysInfoViewWithoutPaths(t *testing.T) {
	s := NewSysInfo()
	out := s.View(tools.Bounds{Width: 80, Height: 24}, tools.NoopRenderer{})
	if !strings.Contains(out, "（未注入）") {
		t.Errorf("未注入路径应显示降级提示，实际: %q", out)
	}
}
