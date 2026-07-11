package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.yaml.in/yaml/v3"
)

// EnvPrefix 是环境变量的统一前缀。
const EnvPrefix = "WORKTIDE"

// LoadResult 是配置加载的结果，包含最终配置与解析出的路径。
type LoadResult struct {
	Config     *Config
	Paths      Paths
	WatchFiles []string
}

// LoadFull 执行完整的配置加载流程：解析目录 -> 合并默认值/文件/环境变量/命令行 -> 校验。
// 返回 LoadResult。文件缺失不视为错误。
func LoadFull() (*LoadResult, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	cfg, watch, err := loadConfig(paths)
	if err != nil {
		return nil, err
	}
	return &LoadResult{Config: cfg, Paths: paths, WatchFiles: watch}, nil
}

// loadConfig 加载并合并多来源配置。
//
// 合并顺序（高 -> 低）：
//   命令行标志 > 环境变量 > services.yaml > include 列表（按顺序） > config.yaml > 内置默认值
func loadConfig(paths Paths) (*Config, []string, error) {
	// 1. 内置默认值（最低优先级）。
	cfg := Default()

	// 2. 读取 config.yaml（如存在）。
	rootRaw := map[string]any{}
	if data, err := os.ReadFile(paths.ConfigFilePath()); err == nil {
		if err := yaml.Unmarshal(data, &rootRaw); err != nil {
			slog.Warn("配置文件解析失败，使用默认配置", "error", err, "path", paths.ConfigFilePath())
			rootRaw = map[string]any{}
		}
	} else if !os.IsNotExist(err) {
		slog.Warn("读取配置文件失败", "error", err, "path", paths.ConfigFilePath())
	}

	// 3. 合并 include 列表（深度合并、循环检测、深度上限）。
	watch := []string{paths.ConfigFilePath()}
	if includeRaw, ok := rootRaw["include"].([]any); ok {
		includePaths := toStringSlice(includeRaw)
		if len(includePaths) > 0 {
			visited := map[string]bool{}
			includeOut, err := mergeIncludes(filepath.Dir(paths.ConfigFilePath()), includePaths, visited, 1)
			if err != nil {
				return nil, nil, err
			}
			if err := deepMerge(rootRaw, includeOut); err != nil {
				return nil, nil, fmt.Errorf("合并 include 失败: %w", err)
			}
			for _, p := range includePaths {
				expanded, _ := expandPath(p)
				if expanded == "" {
					continue
				}
				var abs string
				if filepath.IsAbs(expanded) {
					abs = expanded
				} else {
					abs = filepath.Join(filepath.Dir(paths.ConfigFilePath()), expanded)
				}
				if a, err := filepath.Abs(abs); err == nil {
					watch = append(watch, a)
				}
			}
		}
	}
	// 把 include 字段从 rootRaw 移除，避免被反序列化到 Config。
	delete(rootRaw, "include")

	// 4. 把 rootRaw 合并到 cfg。
	if err := mergeIntoConfig(cfg, rootRaw); err != nil {
		return nil, nil, err
	}

	// 5. 读取 services.yaml（如存在），仅覆盖 services 段。
	if data, err := os.ReadFile(paths.ServicesFile()); err == nil {
		svcs := map[string]any{}
		if err := yaml.Unmarshal(data, &svcs); err != nil {
			slog.Warn("services.yaml 解析失败", "error", err, "path", paths.ServicesFile())
		} else {
			if _, hasInclude := svcs["include"]; hasInclude {
				slog.Warn("services.yaml 含 include 字段，已忽略", "path", paths.ServicesFile())
				delete(svcs, "include")
			}
			if svcMap, ok := svcs["services"].(map[string]any); ok {
				warnOverride(cfg.Services, svcMap)
				if err := mergeIntoConfig(cfg, map[string]any{"services": svcMap}); err != nil {
					return nil, nil, err
				}
			}
		}
		watch = append(watch, paths.ServicesFile())
	} else if !os.IsNotExist(err) {
		slog.Warn("读取 services.yaml 失败", "error", err)
	}

	// 6. 环境变量覆盖。
	applyEnvOverrides(cfg)

	// 7. 命令行标志（阶段 2 暂为占位）。
	bindFlags(cfg)

	// 8. 后处理。
	cfg = postProcess(cfg)

	return cfg, watch, nil
}

// mergeIntoConfig 把 map[string]any 解码到 cfg。
// 使用 yaml.Marshal + yaml.Unmarshal 以复用 ServiceCommand.UnmarshalYAML。
func mergeIntoConfig(cfg *Config, raw map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	// 把 map 序列化为 YAML，再 unmarshal 进 cfg。
	// 这样 ServiceCommand 的 UnmarshalYAML 能正确处理 string/array 双形态。
	intermediate := struct {
		*Config
	}{
		Config: cfg,
	}
	// 先把 raw 合并到 cfg 的零值再统一 unmarshal 更稳。
	// 但 cfg 已经含默认值，重复 Unmarshal 会覆盖。因此仅把 raw 段 unmarshal 到临时结构。
	// 简化：把 raw 序列化为 YAML 后 unmarshal 到同类型 Config，然后按 key 合并到 cfg。
	tmp := Default()
	if err := decodeYAMLMap(raw, tmp); err != nil {
		return err
	}
	// 合并 tmp 到 cfg。
	if err := mergeConfigs(cfg, tmp); err != nil {
		return err
	}
	_ = intermediate
	return nil
}

// decodeYAMLMap 把任意 map[string]any 解码到 dst（通过 YAML 序列化中转）。
func decodeYAMLMap(raw map[string]any, dst *Config) error {
	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("序列化中间配置失败: %w", err)
	}
	// 二次解码：把 YAML 解码到 Config，让 ServiceCommand.UnmarshalYAML 生效。
	return yaml.Unmarshal(data, dst)
}

// mergeConfigs 把 src 中的非零值合并到 dst（in-place）。
// 用于 raw map 解码后的 Config 与 Default 合并。
func mergeConfigs(dst, src *Config) error {
	if src.Include != nil {
		dst.Include = append([]string{}, src.Include...)
	}
	for k, v := range src.Services {
		if dst.Services == nil {
			dst.Services = map[string]ServiceDef{}
		}
		dst.Services[k] = v
	}
	// 标量/简单字段按非零覆盖。
	if src.Theme.Name != "" {
		dst.Theme.Name = src.Theme.Name
	}
	if src.Keymap.Quit != "" {
		dst.Keymap.Quit = src.Keymap.Quit
	}
	if src.Keymap.FocusNav != "" {
		dst.Keymap.FocusNav = src.Keymap.FocusNav
	}
	if src.Keymap.Help != "" {
		dst.Keymap.Help = src.Keymap.Help
	}
	if src.Keymap.Settings != "" {
		dst.Keymap.Settings = src.Keymap.Settings
	}
	if src.Tools.Enabled != nil {
		dst.Tools.Enabled = append([]string{}, src.Tools.Enabled...)
	}
	if src.Backend.Enabled {
		dst.Backend.Enabled = true
	}
	_ = bytes.NewBuffer // 防止 unused import 警告
	return nil
}

// warnOverride 对比 services 中与 cfg.Services 重名的条目并输出 WARN。
func warnOverride(existing map[string]ServiceDef, override map[string]any) {
	for name := range override {
		if _, ok := existing[name]; ok {
			slog.Warn("services.yaml 覆盖 config.yaml 中的服务声明", "name", name)
		}
	}
}

// applyEnvOverrides 应用环境变量覆盖（仅支持简单字段）。
func applyEnvOverrides(cfg *Config) {
	v := os.Getenv(EnvPrefix + "_THEME_NAME")
	if v != "" {
		cfg.Theme.Name = v
	}
	if v := os.Getenv(EnvPrefix + "_KEYMAP_QUIT"); v != "" {
		cfg.Keymap.Quit = v
	}
	if v := os.Getenv(EnvPrefix + "_KEYMAP_FOCUS_NAV"); v != "" {
		cfg.Keymap.FocusNav = v
	}
	if v := os.Getenv(EnvPrefix + "_KEYMAP_HELP"); v != "" {
		cfg.Keymap.Help = v
	}
	if v := os.Getenv(EnvPrefix + "_KEYMAP_SETTINGS"); v != "" {
		cfg.Keymap.Settings = v
	}
}

// bindFlags 绑定命令行标志到 cfg。阶段 2 暂为占位，阶段 3+ 接入完整 flag。
func bindFlags(_ *Config) {
	// 预留：后续接入 pflag/cobra 时在此绑定。
}

// knownToolIDs 是当前已注册的工具 ID 集合。
var knownToolIDs = map[string]bool{
	"welcome":  true,
	"sysinfo":  true,
	"services": true, // 服务管理工具（v1 默认不启用，需显式加入 tools.enabled）
}

// postProcess 对加载后的配置做校验与兜底：
func postProcess(cfg *Config) *Config {
	valid := make([]string, 0, len(cfg.Tools.Enabled))
	for _, id := range cfg.Tools.Enabled {
		if !knownToolIDs[id] {
			slog.Warn("配置引用了未知的工具 ID，已忽略", "tool", id)
			continue
		}
		valid = append(valid, id)
	}
	if len(valid) == 0 {
		slog.Info("工具启用列表为空，启用默认示例工具")
		valid = Default().Tools.Enabled
	}
	cfg.Tools.Enabled = valid
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceDef{}
	}
	return cfg
}

// ---- 向后兼容 ----

// Load 加载配置并返回。
func Load() (*Config, error) {
	res, err := LoadFull()
	if err != nil {
		return Default(), nil
	}
	return res.Config, nil
}

// ---- 热加载支持 ----

// ReloadCallback 在配置热加载完成时被调用，收到新的配置快照。
type ReloadCallback func(*Config)

// Watcher 封装配置文件的变更监听与回调分发。
type Watcher struct {
	paths   Paths
	watch   []string
	mu      sync.Mutex
	cbs     []ReloadCallback
	stopCh  chan struct{}
	stopped bool
}

// NewWatcher 创建一个基于文件监听的配置热加载器。
func NewWatcher(paths Paths) (*Watcher, error) {
	_, watch, err := loadConfig(paths)
	if err != nil {
		watch = []string{paths.ConfigFilePath()}
	}
	return &Watcher{
		paths:  paths,
		watch:  watch,
		stopCh: make(chan struct{}),
	}, nil
}

// OnReload 注册一个热加载回调。
func (w *Watcher) OnReload(cb ReloadCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cbs = append(w.cbs, cb)
}

// Start 开始监听配置文件变更。
func (w *Watcher) Start() error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("创建 fsnotify watcher 失败: %w", err)
	}
	for _, p := range w.watch {
		_ = fsw.Add(p)
	}
	go func() {
		defer fsw.Close()
		for {
			select {
			case ev, ok := <-fsw.Events:
				if !ok {
					return
				}
				slog.Debug("配置文件变更", "event", ev.Op.String(), "file", ev.Name)
				cfg, _, err := loadConfig(w.paths)
				if err != nil {
					slog.Warn("热加载解析失败，忽略本次变更", "error", err)
					continue
				}
				w.mu.Lock()
				cbs := make([]ReloadCallback, len(w.cbs))
				copy(cbs, w.cbs)
				w.mu.Unlock()
				for _, cb := range cbs {
					func() {
						defer func() {
							if r := recover(); r != nil {
								slog.Warn("配置热加载回调 panic", "error", r)
							}
						}()
						cb(cfg)
					}()
				}
			case _, ok := <-fsw.Errors:
				if !ok {
					return
				}
			case <-w.stopCh:
				return
			}
		}
	}()
	return nil
}

// Stop 停止监听。
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.stopCh)
}

// PathsAccessible 返回配置文件是否存在。
func PathsAccessible(paths Paths) bool {
	_, err := os.Stat(paths.ConfigFilePath())
	return err == nil
}
