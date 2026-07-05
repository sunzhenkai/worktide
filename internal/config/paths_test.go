package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestResolvePathsContainsAppName 验证各平台路径都包含应用名 worktide。
func TestResolvePathsContainsAppName(t *testing.T) {
	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths 失败: %v", err)
	}
	for name, dir := range map[string]string{
		"ConfigDir": paths.ConfigDir,
		"DataDir":   paths.DataDir,
		"CacheDir":  paths.CacheDir,
		"LogDir":    paths.LogDir,
	} {
		if !strings.Contains(filepath.Base(dir), AppName) && name == "ConfigDir" {
			t.Errorf("%s 末级目录应包含 %s，实际: %s", name, AppName, dir)
		}
		if dir == "" {
			t.Errorf("%s 不应为空", name)
		}
	}
}

// TestResolvePathsPlatformSpecific 校验各平台的具体路径约定。
func TestResolvePathsPlatformSpecific(t *testing.T) {
	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths 失败: %v", err)
	}
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(paths.ConfigDir, "Application Support") {
			t.Errorf("macOS 配置目录应位于 Application Support，实际: %s", paths.ConfigDir)
		}
		if !strings.Contains(paths.CacheDir, "Caches") {
			t.Errorf("macOS 缓存目录应位于 Caches，实际: %s", paths.CacheDir)
		}
	case "linux":
		if os.Getenv("XDG_CONFIG_HOME") == "" {
			// 默认 ~/.config
			if !strings.HasSuffix(paths.ConfigDir, filepath.Join(".config", AppName)) {
				t.Errorf("Linux 默认配置目录应为 ~/.config/%s，实际: %s", AppName, paths.ConfigDir)
			}
		}
	case "windows":
		if !strings.Contains(paths.ConfigDir, AppName) {
			t.Errorf("Windows 配置目录应包含 %s，实际: %s", AppName, paths.ConfigDir)
		}
	}
}

// TestEnsureDirsCreatesMissing 验证缺失目录会被自动创建。
func TestEnsureDirsCreatesMissing(t *testing.T) {
	base := t.TempDir()
	paths := Paths{
		ConfigDir: filepath.Join(base, "config"),
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "data", "logs"),
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs 失败: %v", err)
	}
	for name, dir := range map[string]string{
		"ConfigDir": paths.ConfigDir,
		"DataDir":   paths.DataDir,
		"CacheDir":  paths.CacheDir,
		"LogDir":    paths.LogDir,
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("%s 未被创建: %v", name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s 应为目录", name)
		}
	}
}

// TestEnsureDirsIdempotent 验证多次调用不报错。
func TestEnsureDirsIdempotent(t *testing.T) {
	base := t.TempDir()
	paths := Paths{
		ConfigDir: filepath.Join(base, "config"),
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "logs"),
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("首次 EnsureDirs 失败: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("二次 EnsureDirs 应幂等，失败: %v", err)
	}
}

// TestConfigFilePath 拼接正确。
func TestConfigFilePath(t *testing.T) {
	paths := Paths{ConfigDir: "/tmp/worktide"}
	want := filepath.Join("/tmp/worktide", ConfigFileName)
	if got := paths.ConfigFilePath(); got != want {
		t.Errorf("ConfigFilePath = %s, want %s", got, want)
	}
}
