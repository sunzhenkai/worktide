package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultHasBuiltinTools 验证默认启用示例工具。
func TestDefaultHasBuiltinTools(t *testing.T) {
	d := Default()
	if len(d.Tools.Enabled) == 0 {
		t.Fatal("默认启用列表不应为空")
	}
	found := map[string]bool{}
	for _, id := range d.Tools.Enabled {
		found[id] = true
	}
	if !found["welcome"] || !found["sysinfo"] {
		t.Errorf("默认应启用 welcome 与 sysinfo，实际: %v", d.Tools.Enabled)
	}
}

// TestLoadFullFallbackOnMissingFile 验证配置文件缺失时使用默认值且不报错。
func TestLoadFullFallbackOnMissingFile(t *testing.T) {
	// 将配置目录指向空临时目录，模拟首次启动。
	base := t.TempDir()
	paths := Paths{
		ConfigDir: base,
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "logs"),
	}
	cfg, _, err := loadConfig(paths)
	if err != nil {
		t.Fatalf("配置缺失时应使用默认值而非报错，实际: %v", err)
	}
	if cfg.Theme.Name != "default" {
		t.Errorf("默认主题应为 default，实际: %s", cfg.Theme.Name)
	}
}

// TestLoadFullReadsFile 验证配置文件值覆盖默认值。
func TestLoadFullReadsFile(t *testing.T) {
	base := t.TempDir()
	paths := Paths{
		ConfigDir: base,
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "logs"),
	}
	// 写入一个覆盖主题的配置文件。
	content := []byte("theme:\n  name: dark\n")
	if err := os.WriteFile(paths.ConfigFilePath(), content, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	cfg, _, err := loadConfig(paths)
	if err != nil {
		t.Fatalf("loadConfig 失败: %v", err)
	}
	if cfg.Theme.Name != "dark" {
		t.Errorf("应从文件读取 theme.name=dark，实际: %s", cfg.Theme.Name)
	}
}

// TestPostProcessUnknownToolWarned 验证未知工具 ID 被过滤。
func TestPostProcessUnknownToolWarned(t *testing.T) {
	cfg := Default()
	cfg.Tools.Enabled = []string{"welcome", "foo", "sysinfo"}
	got := postProcess(cfg)
	found := map[string]bool{}
	for _, id := range got.Tools.Enabled {
		found[id] = true
	}
	if found["foo"] {
		t.Error("未知工具 foo 应被过滤")
	}
	if !found["welcome"] || !found["sysinfo"] {
		t.Error("已知工具应保留")
	}
}

// TestPostProcessEmptyFallback 验证空启用列表回退到默认。
func TestPostProcessEmptyFallback(t *testing.T) {
	cfg := Default()
	cfg.Tools.Enabled = []string{}
	got := postProcess(cfg)
	if len(got.Tools.Enabled) == 0 {
		t.Fatal("空列表应回退到默认示例工具")
	}
}

// TestPostProcessAllUnknownFallback 验证全部未知时回退。
func TestPostProcessAllUnknownFallback(t *testing.T) {
	cfg := Default()
	cfg.Tools.Enabled = []string{"nope1", "nope2"}
	got := postProcess(cfg)
	if len(got.Tools.Enabled) == 0 {
		t.Fatal("全部未知时应回退到默认示例工具")
	}
	for _, id := range got.Tools.Enabled {
		if id != "welcome" && id != "sysinfo" {
			t.Errorf("回退后应仅含默认工具，实际含: %s", id)
		}
	}
}

// TestEnvOverrideFile 验证环境变量优先级高于配置文件。
func TestEnvOverrideFile(t *testing.T) {
	t.Setenv("WORKTIDE_THEME_NAME", "dark")
	base := t.TempDir()
	paths := Paths{
		ConfigDir: base,
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "logs"),
	}
	content := []byte("theme:\n  name: light\n")
	if err := os.WriteFile(paths.ConfigFilePath(), content, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	cfg, _, err := loadConfig(paths)
	if err != nil {
		t.Fatalf("loadConfig 失败: %v", err)
	}
	// 环境变量应覆盖文件值 light。
	if cfg.Theme.Name != "dark" {
		t.Errorf("环境变量应覆盖文件，期望 dark，实际: %s", cfg.Theme.Name)
	}
}

// TestLoadReturnsDefaultOnError 验证 Load 在加载失败时回退默认值。
func TestLoadReturnsDefaultOnError(t *testing.T) {
	// 即使内部出错，Load 也应返回默认值而非 nil+err。
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 不应返回错误（应回退默认值）: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load 不应返回 nil 配置")
	}
	if cfg.Theme.Name != "default" {
		t.Errorf("回退配置应为默认主题，实际: %s", cfg.Theme.Name)
	}
}

// TestPathsAccessible 验证文件存在性检测。
func TestPathsAccessible(t *testing.T) {
	base := t.TempDir()
	paths := Paths{ConfigDir: base}
	if PathsAccessible(paths) {
		t.Error("配置文件不存在时应返回 false")
	}
	if err := os.WriteFile(paths.ConfigFilePath(), []byte("theme:\n  name: x\n"), 0o600); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if !PathsAccessible(paths) {
		t.Error("配置文件存在时应返回 true")
	}
}
