// Package config 负责 WorkTide 的跨平台目录解析、配置加载与默认值管理。
package config

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config 是 WorkTide 的运行时配置根结构。
type Config struct {
	Theme    ThemeConfig             `mapstructure:"theme"`
	Keymap   KeymapConfig            `mapstructure:"keymap"`
	Tools    ToolsConfig             `mapstructure:"tools"`
	Backend  BackendConfig           `mapstructure:"backend"`
	Include  []string                `mapstructure:"include"`
	Services map[string]ServiceDef   `mapstructure:"services"`
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

// ServiceDef 描述一个服务的声明信息（来自 config.yaml / include / services.yaml）。
// 声明仅包含「如何启动」，运行态（PID / PGID / LogPath）由 bbolt 维护。
type ServiceDef struct {
	// Cwd 是服务工作目录。
	Cwd string `yaml:"cwd" mapstructure:"cwd"`
	// Command 是启动命令（shell 字符串或 argv 数组）。
	Command ServiceCommand `yaml:"command" mapstructure:"command"`
	// Description 可选的描述信息。
	Description string `yaml:"description,omitempty" mapstructure:"description,omitempty"`
	// Env 是附加到子进程的环境变量。
	Env map[string]string `yaml:"env,omitempty" mapstructure:"env,omitempty"`
	// ForceRestart 在热加载时若为 true 则提示需要重启。
	ForceRestart bool `yaml:"force_restart,omitempty" mapstructure:"force_restart,omitempty"`
}

// ServiceCommand 支持两种形态：shell 字符串或 argv 数组。
// 通过 UnmarshalYAML 区分。
type ServiceCommand struct {
	// Shell 为 true 时表示按 shell 命令解析（Cmd 中是单一字符串）。
	Shell bool
	// Cmd 包含解释后的命令：
	//   - Shell=true：Cmd[0] 是完整命令字符串
	//   - Shell=false：Cmd 是 argv 数组
	Cmd []string
}

// UnmarshalYAML 实现 shell / argv 双形态解码。
func (s *ServiceCommand) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		s.Shell = true
		s.Cmd = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			return errors.New("command 数组不能为空")
		}
		parts := make([]string, 0, len(node.Content))
		for i, n := range node.Content {
			if n.Kind != yaml.ScalarNode {
				return fmt.Errorf("command 数组第 %d 项必须是字符串", i)
			}
			parts = append(parts, n.Value)
		}
		s.Shell = false
		s.Cmd = parts
		return nil
	default:
		return fmt.Errorf("command 必须是字符串或字符串数组，实际类型: %v", node.Kind)
	}
}

// MarshalYAML 把 ServiceCommand 序列化为字符串或数组。
func (s ServiceCommand) MarshalYAML() (any, error) {
	if s.Shell {
		if len(s.Cmd) == 0 {
			return "", nil
		}
		return s.Cmd[0], nil
	}
	return s.Cmd, nil
}

// String 把命令格式化为可读字符串。
func (s ServiceCommand) String() string {
	if len(s.Cmd) == 0 {
		return ""
	}
	if s.Shell {
		return s.Cmd[0]
	}
	return strings.Join(s.Cmd, " ")
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
		Services: map[string]ServiceDef{},
	}
}
