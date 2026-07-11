package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestManager(t *testing.T) (*Manager, *bolt.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("打开 db 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	dataDir := t.TempDir()
	m, err := NewManager(db, map[string]ServiceDef{}, dataDir)
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}
	return m, db
}

func TestRegistryUpsertGetListDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "r.db")
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("打开 db 失败: %v", err)
	}
	defer db.Close()
	reg, err := Open(db)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}

	r1 := &Record{Name: "a", PID: 123, Cwd: "/x", LastStatus: StatusRunning}
	if err := reg.Upsert(r1); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}
	got, err := reg.Get("a")
	if err != nil || got == nil || got.PID != 123 {
		t.Fatalf("Get 错误: %v / %+v", err, got)
	}
	got.PID = 456
	if err := reg.Upsert(got); err != nil {
		t.Fatalf("Upsert 更新失败: %v", err)
	}
	got2, _ := reg.Get("a")
	if got2.PID != 456 {
		t.Errorf("更新未生效: %d", got2.PID)
	}

	list, err := reg.List()
	if err != nil || len(list) != 1 {
		t.Errorf("List 结果错误: %v %+v", err, list)
	}
	if err := reg.Delete("a"); err != nil {
		t.Errorf("Delete 失败: %v", err)
	}
	got3, _ := reg.Get("a")
	if got3 != nil {
		t.Errorf("Delete 后 Get 应返回 nil")
	}
}

func TestRunKillSleep(t *testing.T) {
	m, _ := newTestManager(t)
	rec, err := m.Run(RunOptions{
		Name:    "sleep1",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 5"}},
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if rec.PID <= 0 {
		t.Errorf("PID 应 > 0，实际: %d", rec.PID)
	}

	// 状态应为 running。
	entry, err := m.Status("sleep1")
	if err != nil {
		t.Fatalf("Status 失败: %v", err)
	}
	if entry.Status != StatusRunning {
		t.Errorf("期望 running，实际: %s", entry.Status)
	}

	// 终止。
	if err := m.Kill("sleep1", false); err != nil {
		t.Fatalf("Kill 失败: %v", err)
	}
	// 再次状态应为 exited/stale。
	time.Sleep(100 * time.Millisecond)
	entry, _ = m.Status("sleep1")
	if entry.Status == StatusRunning {
		t.Errorf("Kill 后应非 running，实际: %s", entry.Status)
	}
}

func TestRunDuplicateRejected(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.Run(RunOptions{
		Name:    "dup",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
	})
	if err != nil {
		t.Fatalf("首次 Run 失败: %v", err)
	}
	_, err2 := m.Run(RunOptions{
		Name:    "dup",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
	})
	if err2 == nil || !strings.Contains(err2.Error(), "already running") {
		t.Errorf("同名应拒绝，实际: %v", err2)
	}
}

func TestRunForceReplaces(t *testing.T) {
	m, _ := newTestManager(t)
	rec1, err := m.Run(RunOptions{
		Name:    "svc",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
	})
	if err != nil {
		t.Fatalf("首次 Run 失败: %v", err)
	}
	rec2, err := m.Run(RunOptions{
		Name:    "svc",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
		Force:   true,
	})
	if err != nil {
		t.Fatalf("force Run 失败: %v", err)
	}
	if rec2.PID == rec1.PID {
		t.Errorf("force 后 PID 应不同：%d vs %d", rec1.PID, rec2.PID)
	}
	// 清理。
	_ = m.Kill("svc", true)
}

func TestListReturnsDeclaredAndRunning(t *testing.T) {
	m, _ := newTestManager(t)
	// 启动一个服务。
	_, err := m.Run(RunOptions{
		Name:    "alive",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	defer m.Kill("alive", true)

	// 加一个仅声明的（未启动）。
	m.UpdateDecl("declared", ServiceDef{Cwd: "/d", Command: ServiceCommand{Shell: true, Cmd: []string{"x"}}})

	list, err := m.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	got := map[string]Status{}
	for _, e := range list {
		got[e.Name] = e.Status
	}
	if got["alive"] != StatusRunning {
		t.Errorf("alive 状态错误: %s", got["alive"])
	}
	if got["declared"] != StatusStale {
		t.Errorf("declared 状态应为 stale，实际: %s", got["declared"])
	}
}

func TestClean(t *testing.T) {
	m, _ := newTestManager(t)
	// 启动一个 sleep 进程。
	_, err := m.Run(RunOptions{
		Name:    "tmp",
		Cwd:     t.TempDir(),
		Command: ServiceCommand{Shell: true, Cmd: []string{"sleep 3"}},
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	defer m.Kill("tmp", true)

	n, err := m.Clean(CleanOptions{})
	if err != nil {
		t.Fatalf("Clean 失败: %v", err)
	}
	if n != 0 {
		t.Errorf("running 不应被清理，实际清理 %d", n)
	}
	// 终止后再清理。
	_ = m.Kill("tmp", true)
	time.Sleep(100 * time.Millisecond)
	n2, err := m.Clean(CleanOptions{})
	if err != nil {
		t.Fatalf("Clean 二次失败: %v", err)
	}
	if n2 != 1 {
		t.Errorf("exited 应被清理，实际清理 %d", n2)
	}
}

func TestRestartRequiresDecl(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.Restart("unknown")
	if err == nil {
		t.Error("未声明的服务 Restart 应报错")
	}
}

func TestRemoveDeclOnlyAffectsDecl(t *testing.T) {
	m, _ := newTestManager(t)
	m.UpdateDecl("foo", ServiceDef{Cwd: "/x", Command: ServiceCommand{Shell: true, Cmd: []string{"y"}}})
	if err := m.RemoveDecl("foo"); err != nil {
		t.Fatalf("RemoveDecl 失败: %v", err)
	}
	if _, ok := m.Decls()["foo"]; ok {
		t.Error("RemoveDecl 后声明应消失")
	}
}

func TestReadTailLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(p, []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadTailLines(p, 3)
	if err != nil {
		t.Fatalf("ReadTailLines 失败: %v", err)
	}
	if len(lines) != 3 || lines[0] != "c" || lines[2] != "e" {
		t.Errorf("tail 结果错误: %v", lines)
	}
}

func TestReadTailLinesEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing.txt")
	lines, err := ReadTailLines(p, 10)
	if err != nil || lines != nil {
		t.Errorf("缺失文件应返回 (nil, nil)，实际: %v %v", lines, err)
	}
}
