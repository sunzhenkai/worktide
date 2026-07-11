package service

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Manager 统一管理服务声明、运行态与日志。
// 线程安全；CLI 与 TUI 共用同一实例。
type Manager struct {
	db      *bolt.DB
	reg     *Registry
	mu      sync.Mutex
	decls   map[string]ServiceDef
	dataDir string
	logsDir string
}

// NewManager 构造 Manager。
//   - db: 已打开的 bbolt 实例（生命周期由调用方管理）
//   - decls: 服务声明（通常来自 config.Services）
//   - dataDir: 数据目录（日志存放于 dataDir/services/logs/）
func NewManager(db *bolt.DB, decls map[string]ServiceDef, dataDir string) (*Manager, error) {
	if db == nil {
		return nil, fmt.Errorf("db 不能为空")
	}
	if decls == nil {
		decls = map[string]ServiceDef{}
	}
	reg, err := Open(db)
	if err != nil {
		return nil, err
	}
	logsDir := filepath.Join(dataDir, "services", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	return &Manager{
		db:      db,
		reg:     reg,
		decls:   decls,
		dataDir: dataDir,
		logsDir: logsDir,
	}, nil
}

// Decls 返回当前声明的快照。
func (m *Manager) Decls() map[string]ServiceDef {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]ServiceDef, len(m.decls))
	for k, v := range m.decls {
		out[k] = v
	}
	return out
}

// UpdateDecl 更新单条声明（热加载时使用）。
func (m *Manager) UpdateDecl(name string, def ServiceDef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.decls == nil {
		m.decls = map[string]ServiceDef{}
	}
	m.decls[name] = def
}

// SetDecls 整体替换声明。
func (m *Manager) SetDecls(decls map[string]ServiceDef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if decls == nil {
		decls = map[string]ServiceDef{}
	}
	m.decls = decls
}

// LogPath 返回服务日志文件路径。
func (m *Manager) LogPath(name string) string {
	return filepath.Join(m.logsDir, name+".log")
}

// ---- 运行态查询 ----

// Status 返回单条服务的当前状态。
func (m *Manager) Status(name string) (*Entry, error) {
	rec, err := m.reg.Get(name)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	decl, hasDecl := m.decls[name]
	m.mu.Unlock()
	entry := &Entry{Name: name}
	if hasDecl {
		d := decl
		entry.Decl = &d
	}
	if rec == nil {
		entry.Status = StatusStale
		return entry, nil
	}
	entry.Record = rec
	if isProcessAlive(rec.PID, rec.PGID) {
		entry.Status = StatusRunning
		rec.LastStatus = StatusRunning
	} else {
		// 上次为 running 但进程已退出 → exited
		if rec.LastStatus == StatusRunning {
			entry.Status = StatusExited
		} else {
			entry.Status = StatusStale
		}
		rec.LastStatus = entry.Status
		// 持久化更新后的状态（best-effort）。
		_ = m.reg.Upsert(rec)
	}
	return entry, nil
}

// List 返回所有服务的 Entry（包含声明 + 运行态）。
// 列表项按 Name 排序。
func (m *Manager) List() ([]*Entry, error) {
	records, err := m.reg.List()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	decls := make(map[string]ServiceDef, len(m.decls))
	for k, v := range m.decls {
		decls[k] = v
	}
	m.mu.Unlock()

	byName := map[string]*Record{}
	for _, r := range records {
		byName[r.Name] = r
	}
	// 合并：先遍历所有声明，再遍历运行态。
	seen := map[string]bool{}
	out := make([]*Entry, 0)
	for name, decl := range decls {
		seen[name] = true
		d := decl
		e := &Entry{Name: name, Decl: &d}
		if rec, ok := byName[name]; ok {
			e.Record = rec
			if isProcessAlive(rec.PID, rec.PGID) {
				e.Status = StatusRunning
				rec.LastStatus = StatusRunning
			} else {
				if rec.LastStatus == StatusRunning {
					e.Status = StatusExited
				} else {
					e.Status = StatusStale
				}
				rec.LastStatus = e.Status
				_ = m.reg.Upsert(rec)
			}
		} else {
			e.Status = StatusStale
		}
		out = append(out, e)
	}
	for name, rec := range byName {
		if seen[name] {
			continue
		}
		e := &Entry{Name: name, Record: rec}
		if isProcessAlive(rec.PID, rec.PGID) {
			e.Status = StatusRunning
		} else if rec.LastStatus == StatusRunning {
			e.Status = StatusExited
		} else {
			e.Status = StatusStale
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ---- 运行态操作 ----

// RunOptions 描述一次启动请求。
type RunOptions struct {
	Name    string
	Cwd     string
	Command ServiceCommand
	Env     map[string]string
	Force   bool
}

// Run 启动一个服务并写入运行态。
//   - 若同名服务正在运行且 !opts.Force，返回错误；
//   - 若 opts.Force 为 true 且原服务在运行，先发送 SIGTERM 终止。
func (m *Manager) Run(opts RunOptions) (*Record, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("服务名不能为空")
	}
	if len(opts.Command.Cmd) == 0 {
		return nil, fmt.Errorf("命令不能为空")
	}

	// 检查同名已运行。
	if existing, err := m.reg.Get(opts.Name); err != nil {
		return nil, err
	} else if existing != nil && isProcessAlive(existing.PID, existing.PGID) {
		if !opts.Force {
			return nil, fmt.Errorf("service %q is already running (pid %d)", opts.Name, existing.PID)
		}
		// --force：先终止。
		if err := signalProcess(existing.PID, existing.PGID, syscall.SIGTERM); err != nil {
			slog.Warn("force kill 旧进程失败", "name", opts.Name, "error", err)
		}
		// 短暂等待进程退出。
		for i := 0; i < 20; i++ {
			if !isProcessAlive(existing.PID, existing.PGID) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	// 解析 cwd（默认当前目录）。
	cwd := opts.Cwd
	if cwd == "" {
		if d, err := os.Getwd(); err == nil {
			cwd = d
		}
	}
	logPath := m.LogPath(opts.Name)
	cmd, f, err := startCommand(cwd, opts.Command, logPath, opts.Env)
	if err != nil {
		return nil, err
	}

	// 启动进程。
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("启动进程失败: %w", err)
	}
	// 关闭父进程持有的日志句柄（子进程已继承 fd，重复关闭不会影响子进程写入）。
	_ = f.Close()

	rec := &Record{
		Name:      opts.Name,
		PID:       cmd.Process.Pid,
		PGID:      platformPGID(cmd),
		Cwd:       cwd,
		Command:   opts.Command,
		LogPath:   logPath,
		StartedAt: time.Now(),
		LastStatus: StatusRunning,
	}
	if err := m.reg.Upsert(rec); err != nil {
		return nil, fmt.Errorf("写入运行态失败: %w", err)
	}
	// 异步回收：进程退出后清理（best-effort）。
	go m.reapOnExit(cmd, opts.Name, rec)
	return rec, nil
}

// Kill 终止服务（默认 SIGTERM；force=true 使用 SIGKILL）。
func (m *Manager) Kill(name string, force bool) error {
	rec, err := m.reg.Get(name)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("service %q not found", name)
	}
	if !isProcessAlive(rec.PID, rec.PGID) {
		return fmt.Errorf("service %q 未在运行", name)
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := signalProcess(rec.PID, rec.PGID, sig); err != nil {
		return err
	}
	// 等待退出。
	for i := 0; i < 40; i++ {
		if !isProcessAlive(rec.PID, rec.PGID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	rec.LastStatus = StatusExited
	_ = m.reg.Upsert(rec)
	return nil
}

// Restart 重启服务：先 kill 再 Run。
func (m *Manager) Restart(name string) (*Record, error) {
	m.mu.Lock()
	decl, hasDecl := m.decls[name]
	m.mu.Unlock()
	if !hasDecl {
		return nil, fmt.Errorf("service %q has no command to restart", name)
	}
	// 尝试 kill（不在运行也忽略）。
	_ = m.Kill(name, true)
	return m.Run(RunOptions{
		Name:    name,
		Cwd:     decl.Cwd,
		Command: decl.Command,
		Env:     decl.Env,
	})
}

// CleanOptions 控制清理行为。
type CleanOptions struct {
	// All 同时移除 running 状态的记录（不杀进程）。
	All bool
	// Logs 同时删除对应的日志文件。
	Logs bool
}

// Clean 清理 exited/stale 记录；All=true 时包含 running。
func (m *Manager) Clean(opts CleanOptions) (int, error) {
	recs, err := m.reg.List()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, r := range recs {
		keep := false
		if !opts.All && isProcessAlive(r.PID, r.PGID) {
			keep = true
		}
		if keep {
			continue
		}
		if err := m.reg.Delete(r.Name); err != nil {
			return removed, err
		}
		if opts.Logs {
			_ = os.Remove(r.LogPath)
		}
		removed++
	}
	return removed, nil
}

// RemoveDecl 从声明中移除（仅影响声明，不终止运行中的进程）。
func (m *Manager) RemoveDecl(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.decls[name]; !ok {
		return fmt.Errorf("service %q 未在声明中", name)
	}
	delete(m.decls, name)
	return nil
}

// ---- 辅助 ----

// reapOnExit 等待进程退出并更新 LastStatus。
func (m *Manager) reapOnExit(cmd *exec.Cmd, name string, rec *Record) {
	_ = cmd.Wait()
	if rec == nil {
		return
	}
	cur, err := m.reg.Get(name)
	if err != nil || cur == nil {
		return
	}
	if !isProcessAlive(cur.PID, cur.PGID) {
		cur.LastStatus = StatusExited
		if cur.PID == rec.PID {
			es := cmd.ProcessState.ExitCode()
			cur.ExitStatus = &es
		}
		_ = m.reg.Upsert(cur)
	}
}

// platformPGID 在 Unix 上返回 cmd.Process.Pid（Setpgid 后 PGID=PID），在 Windows 上返回 0。
func platformPGID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	if pgid, ok := pgidFromCmd(cmd); ok {
		return pgid
	}
	return cmd.Process.Pid
}
