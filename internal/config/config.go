// Package config 负责 WorkTide 的跨平台目录解析、配置加载与默认值管理。
//
// 阶段 1 仅提供最小骨架（Config 结构 + 返回默认值的 Load），保证应用可构建。
// 完整的目录解析、Viper 合并加载与热加载将在阶段 2 实现。
package config

// Config 是 WorkTide 的运行时配置根结构。
type Config struct {
	Theme   ThemeConfig   `mapstructure:"theme"`
	Keymap  KeymapConfig  `mapstructure:"keymap"`
	Tools   ToolsConfig   `mapstructure:"tools"`
	Backend BackendConfig `mapstructure:"backend"`
}

// ThemeConfig 定义主题相关配置。
type ThemeConfig struct {
	// Name 指定主题名称，如 "default"、"dark"。
	Name string `mapstructure:"name"`
}

// KeymapConfig 定义全局快捷键绑定。
type KeymapConfig struct {
	Quit     string `mapstructure:"quit"`
	FocusNav string `mapstructure:"focus_nav"`
	Help     string `mapstructure:"help"`
	Settings string `mapstructure:"settings"`
}

// ToolsConfig 定义工具加载策略。
type ToolsConfig struct {
	// Enabled 为启用工具 ID 列表；为空时启用默认示例工具。
	Enabled []string `mapstructure:"enabled"`
}

// BackendConfig 定义本地后端开关。
type BackendConfig struct {
	// Enabled 控制是否启动本地后端服务。
	Enabled bool `mapstructure:"enabled"`
}

// Default 返回内置默认配置。当配置文件缺失或字段未设置时使用。
func Default() *Config {
	return &Config{
		Theme: ThemeConfig{
			Name: "default",
		},
		Keymap: KeymapConfig{
			Quit:     "q",
			FocusNav: "tab",
			Help:     "?",
			Settings: "s",
		},
		Tools: ToolsConfig{
			Enabled: []string{"welcome", "sysinfo"},
		},
		Backend: BackendConfig{
			Enabled: true,
		},
	}
}

// 注意：完整的 Load / LoadFull 实现见 loader.go。
