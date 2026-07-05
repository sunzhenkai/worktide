package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Registry 是工具的注册中心与生命周期调度器。
//
// 职责：
//   - 接收工具的显式注册，校验 ID 唯一性与命名规范；
//   - 按启用列表过滤，提供「已启用工具」的有序视图；
//   - 调度激活态：同一时刻仅一个工具激活，切换先 Deactivate 旧再 Activate 新；
//   - 路由按键事件到当前激活工具。
type Registry struct {
	mu sync.RWMutex

	// all 按注册顺序保存所有已注册工具。
	all []Tool
	// byID 提供 ID -> 工具的快速索引。
	byID map[string]Tool

	// enabled 由 applyEnabled 设置，仅含启用且初始化成功的工具。
	enabled []Tool

	// active 是当前激活工具的 ID（"" 表示无激活）。
	activeID string
	// activeTool 是当前激活工具的缓存指针（无需再次查找）。
	activeTool Tool

	// initialized 记录已完成 Init 的工具 ID，避免重复初始化。
	initialized map[string]bool
}

// NewRegistry 创建一个空的注册中心。
func NewRegistry() *Registry {
	return &Registry{
		byID:        make(map[string]Tool),
		initialized: make(map[string]bool),
	}
}

// Register 将工具登记到注册中心。
// 返回错误的情况：
//   - ID 不符合 kebab-case；
//   - ID 已存在（ErrDuplicateID）。
//
// 注册不会初始化工具；初始化发生在 ApplyEnabled 时。
func (r *Registry) Register(t Tool) error {
	meta := t.Meta()
	if err := ValidateID(meta.ID); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[meta.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateID, meta.ID)
	}

	r.all = append(r.all, t)
	r.byID[meta.ID] = t
	slog.Info("工具已注册", "id", meta.ID, "name", meta.Name, "version", meta.Version)
	return nil
}

// All 返回所有已注册工具元信息的有序副本。
func (r *Registry) All() []Meta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Meta, 0, len(r.all))
	for _, t := range r.all {
		out = append(out, t.Meta())
	}
	return out
}

// ApplyEnabled 根据启用列表（工具 ID）筛选、初始化工具。
//   - 启用列表为空时，回退到所有已注册工具（由调用方在 config 层做默认兜底）。
//   - 引用未注册 ID 的条目被记录为警告并跳过。
//   - 初始化失败的工具被记录为错误并跳过，不影响其他工具。
//
// 返回成功启用的工具 ID 列表。
func (r *Registry) ApplyEnabled(ctx context.Context, enabledIDs []string) ([]string, error) {
	r.mu.Lock()
	// 先释放当前已初始化的工具（若有），以便重新应用。
	r.unlockActiveLocked()
	// 仅关闭之前真正初始化过的工具，避免对未初始化工具调用 Close。
	allTools := make([]Tool, 0, len(r.all))
	for _, t := range r.all {
		if r.initialized[t.Meta().ID] {
			allTools = append(allTools, t)
		} else {
			// 未初始化的工具也加入快照（供空列表回退使用），但不参与释放。
			allTools = append(allTools, t)
		}
	}
	initializedSnapshot := make(map[string]bool, len(r.initialized))
	for k, v := range r.initialized {
		initializedSnapshot[k] = v
	}
	r.mu.Unlock()

	// 仅释放之前真正初始化过的工具。
	for _, t := range allTools {
		if initializedSnapshot[t.Meta().ID] {
			_ = t.Close() // 尽力释放，忽略错误。
		}
	}

	r.mu.Lock()
	r.enabled = r.enabled[:0]
	r.initialized = make(map[string]bool)
	r.activeID = ""
	r.activeTool = nil
	r.mu.Unlock()

	// 计算需要启用的工具集合（保持启用列表顺序，未列出的不启用）。
	want := make([]Tool, 0, len(enabledIDs))
	if len(enabledIDs) == 0 {
		want = append(want, allTools...)
	} else {
		for _, id := range enabledIDs {
			t, ok := r.byID[id]
			if !ok {
				slog.Warn("启用列表引用了未注册的工具，已忽略", "id", id)
				continue
			}
			want = append(want, t)
		}
	}

	// 依次初始化。
	enabledOK := make([]string, 0, len(want))
	for _, t := range want {
		meta := t.Meta()
		if err := t.Init(ctx); err != nil {
			slog.Error("工具初始化失败，已跳过", "id", meta.ID, "error", err)
			continue
		}
		r.mu.Lock()
		r.enabled = append(r.enabled, t)
		r.initialized[meta.ID] = true
		r.mu.Unlock()
		enabledOK = append(enabledOK, meta.ID)
		slog.Info("工具已启用", "id", meta.ID)
	}

	return enabledOK, nil
}

// Enabled 返回当前已启用（且初始化成功）的工具元信息有序副本。
func (r *Registry) Enabled() []Meta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Meta, 0, len(r.enabled))
	for _, t := range r.enabled {
		out = append(out, t.Meta())
	}
	return out
}

// EnabledTools 返回已启用工具实例的有序快照（供 UI 层适配使用）。
func (r *Registry) EnabledTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, len(r.enabled))
	copy(out, r.enabled)
	return out
}

// Activate 激活指定 ID 的工具。
//   - 若当前已有其他工具激活，先 Deactivate 之；
//   - 若目标与当前激活工具相同，则幂等返回 nil；
//   - 目标未启用或初始化失败时返回 ErrUnknownTool。
func (r *Registry) Activate(ctx context.Context, id string) error {
	r.mu.RLock()
	target, ok := r.byID[id]
	enabled := false
	for _, t := range r.enabled {
		if t.Meta().ID == id {
			enabled = true
			break
		}
	}
	r.mu.RUnlock()

	if !ok || !enabled {
		return fmt.Errorf("%w: %s", ErrUnknownTool, id)
	}

	r.mu.Lock()
	// 先停用旧工具（在持锁外执行，避免在 Activate 内回调造成死锁）。
	prev := r.activeTool
	prevID := r.activeID
	r.activeTool = nil
	r.activeID = ""
	r.mu.Unlock()

	if prev != nil && prevID != id {
		if err := prev.Deactivate(); err != nil {
			slog.Warn("工具 Deactivate 失败", "id", prevID, "error", err)
		}
	}

	if err := target.Activate(ctx); err != nil {
		return fmt.Errorf("激活工具 %s 失败: %w", id, err)
	}

	r.mu.Lock()
	r.activeTool = target
	r.activeID = id
	r.mu.Unlock()

	slog.Info("工具已激活", "id", id)
	return nil
}

// DeactivateActive 停用当前激活工具（若存在）。
func (r *Registry) DeactivateActive() error {
	r.mu.Lock()
	prev := r.activeTool
	prevID := r.activeID
	r.activeTool = nil
	r.activeID = ""
	r.mu.Unlock()

	if prev == nil {
		return nil
	}
	if err := prev.Deactivate(); err != nil {
		slog.Warn("工具 Deactivate 失败", "id", prevID, "error", err)
		return err
	}
	return nil
}

// ActiveID 返回当前激活工具的 ID（无激活时返回 ""）。
func (r *Registry) ActiveID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeID
}

// ActiveTool 返回当前激活工具实例（无激活时返回 nil）。
func (r *Registry) ActiveTool() Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeTool
}

// DispatchKey 将按键事件路由到当前激活工具。
// 无激活工具时返回零值 Result（不产生副作用）。
func (r *Registry) DispatchKey(key Key) Result {
	r.mu.RLock()
	t := r.activeTool
	r.mu.RUnlock()
	if t == nil {
		return Result{}
	}
	// 单个工具的 panic 不应影响调度。
	var result Result
	func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Warn("工具 HandleKey panic，已隔离", "id", t.Meta().ID, "panic", p)
				result = Result{Effect: EffectRerender}
			}
		}()
		result = t.HandleKey(key)
	}()
	return result
}

// ViewActive 渲染当前激活工具视图。无激活工具时返回空串。
func (r *Registry) ViewActive(bounds Bounds, renderer Renderer) string {
	r.mu.RLock()
	t := r.activeTool
	r.mu.RUnlock()
	if t == nil {
		return ""
	}
	var view string
	func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Warn("工具 View panic，已隔离", "id", t.Meta().ID, "panic", p)
				view = fmt.Sprintf("工具 %s 渲染失败", t.Meta().ID)
			}
		}()
		view = t.View(bounds, renderer)
	}()
	return view
}

// CloseAll 释放所有已初始化工具的资源。应用退出时调用。
func (r *Registry) CloseAll() error {
	r.mu.Lock()
	r.activeTool = nil
	r.activeID = ""
	toClose := make([]Tool, len(r.all))
	copy(toClose, r.all)
	r.mu.Unlock()

	var firstErr error
	for _, t := range toClose {
		meta := t.Meta()
		if err := t.Close(); err != nil {
			slog.Warn("工具 Close 失败", "id", meta.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// unlockActiveLocked 是内部辅助：在持锁状态下清空激活引用（不调用 Deactivate）。
// 用于 ApplyEnabled 重置场景。调用方必须持有 r.mu。
//
//nolint:unused // 语义保留，便于后续扩展
func (r *Registry) unlockActiveLocked() {
	r.activeTool = nil
	r.activeID = ""
}
