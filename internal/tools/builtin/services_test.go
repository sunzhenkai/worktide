package builtin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/service"
	"github.com/sunzhenkai/worktide/internal/tools"
)

// newServicesTest 创建一个 Services Tool 与 mgr。
func newServicesTest(t *testing.T) (*Services, *service.Manager) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("打开 db 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	dataDir := t.TempDir()
	mgr, err := service.NewManager(db, nil, dataDir)
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}
	paths := config.Paths{
		ConfigDir: filepath.Join(dataDir, "config"),
		DataDir:   dataDir,
		LogDir:    filepath.Join(dataDir, "logs"),
	}
	return NewServices(mgr, paths), mgr
}

// TestServicesMeta 校验元信息。
func TestServicesMeta(t *testing.T) {
	s := NewServices(nil, config.Paths{})
	m := s.Meta()
	if m.ID != "services" {
		t.Errorf("ID 应为 services，实际: %s", m.ID)
	}
}

// TestServicesLifecycle 校验生命周期不报错。
func TestServicesLifecycle(t *testing.T) {
	s, _ := newServicesTest(t)
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

// TestServicesViewEmpty 校验空列表渲染。
func TestServicesViewEmpty(t *testing.T) {
	s, _ := newServicesTest(t)
	out := s.View(tools.Bounds{Width: 80, Height: 24}, tools.NoopRenderer{})
	if !strings.Contains(out, "服务") {
		t.Errorf("视图应包含「服务」标题: %s", out)
	}
	if !strings.Contains(out, "no services") {
		t.Errorf("空状态应包含提示: %s", out)
	}
}

// TestServicesViewWithRunning 校验含运行服务的渲染。
func TestServicesViewWithRunning(t *testing.T) {
	s, m := newServicesTest(t)
	_, err := m.Run(service.RunOptions{
		Name:    "demo",
		Cwd:     t.TempDir(),
		Command: service.ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	defer m.Kill("demo", true)

	s.refreshList()
	s.refreshPreview()
	out := s.View(tools.Bounds{Width: 80, Height: 24}, tools.NoopRenderer{})
	if !strings.Contains(out, "demo") {
		t.Errorf("视图应包含服务名 demo: %s", out)
	}
}

// TestServicesKeyRouting 校验按键路由。
func TestServicesKeyRouting(t *testing.T) {
	s, _ := newServicesTest(t)
	r := s.HandleKey(tools.Key{Type: tools.KeyRune, Rune: '?'})
	if r.Effect != tools.EffectRerender {
		t.Errorf("? 应触发 Rerender")
	}
	s.mu.Lock()
	if !s.showingHelp {
		t.Error("? 后应进入帮助")
	}
	s.mu.Unlock()
	s.HandleKey(tools.Key{Type: tools.KeyRune, Rune: '?'})
	s.mu.Lock()
	if s.showingHelp {
		t.Error("再次 ? 后应退出帮助")
	}
	s.mu.Unlock()
	// j / k 移动光标（空列表时不会越界）
	s.HandleKey(tools.Key{Type: tools.KeyRune, Rune: 'j'})
	s.HandleKey(tools.Key{Type: tools.KeyRune, Rune: 'k'})
}

// TestServicesRefresh 校验 refreshList 不报错。
func TestServicesRefresh(t *testing.T) {
	s, m := newServicesTest(t)
	_, _ = m.Run(service.RunOptions{
		Name:    "foo",
		Cwd:     t.TempDir(),
		Command: service.ServiceCommand{Shell: true, Cmd: []string{"sleep 2"}},
	})
	defer m.Kill("foo", true)
	s.refreshList()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.list) == 0 {
		t.Error("refresh 后 list 应非空")
	}
}

var _ = fmt.Sprintf
