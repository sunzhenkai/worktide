package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// EnvPrefix 是环境变量的统一前缀。
const EnvPrefix = "WORKTIDE"

// LoadResult 是配置加载的结果，包含最终配置与解析出的路径。
type LoadResult struct {
	Config *Config
	Paths  Paths
}

// LoadFull 执行完整的配置加载流程：解析目录 -> 合并默认值/文件/环境变量/命令行 -> 校验。
// 返回 LoadResult。文件缺失不视为错误。
func LoadFull() (*LoadResult, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(paths)
	if err != nil {
		return nil, err
	}
	return &LoadResult{Config: cfg, Paths: paths}, nil
}

// loadConfig 使用 viper 合并多来源配置并解码到 Config。
//
// 优先级（高 -> 低）：命令行标志 > 环境变量 > 配置文件 > 内置默认值。
// 注意：阶段 1 的 Load() 仅返回默认值；本函数提供完整实现。
func loadConfig(paths Paths) (*Config, error) {
	v := viper.New()

	// 1. 内置默认值（最低优先级）。
	setDefaults(v)

	// 2. 配置文件（缺失不报错）。
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(paths.ConfigDir)
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			// 文件存在但解析失败：记录警告，继续使用默认值。
			slog.Warn("配置文件解析失败，使用默认配置", "error", err, "path", paths.ConfigFilePath())
		}
	}

	// 3. 环境变量（自动绑定 WORKTIDE_<KEY>，嵌套用 _ 分隔）。
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 4. 命令行标志（最高优先级）。
	//    阶段 2 暂不接入完整 flag 解析，仅预留 hook。
	bindFlags(v)

	// 5. 解码到结构体。
	cfg := Default()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("配置解码失败: %w", err)
	}

	// 6. 后处理：未知工具 ID 告警、空启用列表兜底（兜底由 isKnownTool 配合）。
	cfg = postProcess(cfg)

	return cfg, nil
}

// setDefaults 把内置默认值写入 viper。
func setDefaults(v *viper.Viper) {
	d := Default()
	v.SetDefault("theme.name", d.Theme.Name)
	v.SetDefault("keymap.quit", d.Keymap.Quit)
	v.SetDefault("keymap.focus_nav", d.Keymap.FocusNav)
	v.SetDefault("keymap.help", d.Keymap.Help)
	v.SetDefault("keymap.settings", d.Keymap.Settings)
	v.SetDefault("tools.enabled", d.Tools.Enabled)
	v.SetDefault("backend.enabled", d.Backend.Enabled)
}

// bindFlags 绑定命令行标志到 viper。阶段 2 暂为占位，阶段 3+ 接入完整 flag。
func bindFlags(_ *viper.Viper) {
	// 预留：后续接入 pflag/cobra 时在此绑定。
}

// knownToolIDs 是当前已注册的工具 ID 集合。
// 阶段 2 暂用静态集合；阶段 4 将由注册中心动态填充。
var knownToolIDs = map[string]bool{
	"welcome": true,
	"sysinfo": true,
}

// postProcess 对加载后的配置做校验与兜底：
//   - 过滤未知工具 ID 并记录警告；
//   - 启用列表为空时回退到默认示例工具。
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
	return cfg
}

// ---- 向后兼容：阶段 1 的 Load() 保持签名不变 ----

// Load 加载配置并返回。保留阶段 1 的简单签名供 app.Run 使用。
// 内部委托给 LoadFull，失败时回退到默认值。
func Load() (*Config, error) {
	res, err := LoadFull()
	if err != nil {
		// 加载失败时使用默认值，不向调用方报错（符合 spec：缺失仍能启动）。
		return Default(), nil
	}
	return res.Config, nil
}

// ---- 热加载支持 ----

// ReloadCallback 在配置热加载完成时被调用，收到新的配置快照。
type ReloadCallback func(*Config)

// Watcher 封装配置文件的变更监听与回调分发。
type Watcher struct {
	v       *viper.Viper
	paths   Paths
	mu      sync.Mutex
	cbs     []ReloadCallback
	stopCh  chan struct{}
	stopped bool
}

// NewWatcher 创建一个基于文件监听的配置热加载器。
// 创建后调用 Start 开始监听，Stop 结束监听。
func NewWatcher(paths Paths) (*Watcher, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(paths.ConfigDir)
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("读取配置失败: %w", err)
		}
		// 文件不存在：仍创建 watcher，待文件出现后再触发。
	}
	return &Watcher{
		v:      v,
		paths:  paths,
		stopCh: make(chan struct{}),
	}, nil
}

// OnReload 注册一个热加载回调。回调在每次配置文件变更后收到新快照。
func (w *Watcher) OnReload(cb ReloadCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cbs = append(w.cbs, cb)
}

// Start 开始监听配置文件变更。阻塞调用者应放在 goroutine 中。
// 仅主题、快捷键等可热加载项会触发回调；后端等需重启的项由调用方自行判断提示。
func (w *Watcher) Start() error {
	w.v.OnConfigChange(func(_ fsnotify.Event) {
		cfg := Default()
		if err := w.v.Unmarshal(cfg); err != nil {
			slog.Warn("热加载解码失败，忽略本次变更", "error", err)
			return
		}
		cfg = postProcess(cfg)
		w.mu.Lock()
		cbs := make([]ReloadCallback, len(w.cbs))
		copy(cbs, w.cbs)
		w.mu.Unlock()
		for _, cb := range cbs {
			// 单个回调的 panic 不应影响其他回调。
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Warn("配置热加载回调 panic", "error", r)
					}
				}()
				cb(cfg)
			}()
		}
	})
	w.v.WatchConfig()
	return nil
}

// Stop 停止监听。可多次调用。
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.stopCh)
	// viper 内部 watcher 随应用退出自然停止，这里仅标记。
}

// PathsAccessible 返回配置文件是否存在（用于区分首次启动）。
func PathsAccessible(paths Paths) bool {
	_, err := os.Stat(paths.ConfigFilePath())
	return err == nil
}
