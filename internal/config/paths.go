package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// AppName 是应用在文件系统中的目录名。
const AppName = "worktide"

// Paths 表示 WorkTide 在用户机器上的关键目录。
type Paths struct {
	// ConfigDir 存放配置文件（如 config.yaml）。
	ConfigDir string
	// DataDir 存放应用数据（数据库、状态等）。
	DataDir string
	// CacheDir 存放可重建的缓存。
	CacheDir string
	// LogDir 存放日志文件。
	LogDir string
}

// ConfigFileName 是默认配置文件名。
const ConfigFileName = "config.yaml"

// ConfigFilePath 返回默认配置文件的完整路径。
func (p Paths) ConfigFilePath() string {
	return filepath.Join(p.ConfigDir, ConfigFileName)
}

// ResolvePaths 根据当前操作系统解析 WorkTide 的关键目录。
//
// 解析遵循各平台约定：
//   - macOS:  配置/数据位于 ~/Library/Application Support，缓存在 ~/Library/Caches
//   - Linux:  遵循 XDG（XDG_CONFIG_HOME / XDG_DATA_HOME / XDG_CACHE_HOME）
//   - Windows: 位于 %AppData% / %LocalAppData%
//
// 所有目录使用标准库 os.User* 系列函数解析，保证跨平台一致性。
func ResolvePaths() (Paths, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("解析配置目录失败: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("解析用户主目录失败: %w", err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, fmt.Errorf("解析缓存目录失败: %w", err)
	}

	// 数据目录：优先 XDG_DATA_HOME（Linux），否则回退到配置目录下的 data。
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(homeDir, ".local", "share", AppName)
	} else {
		dataDir = filepath.Join(dataDir, AppName)
	}

	return Paths{
		ConfigDir: filepath.Join(cfgDir, AppName),
		DataDir:   dataDir,
		CacheDir:  filepath.Join(cacheDir, AppName),
		LogDir:    filepath.Join(dataDir, "logs"),
	}, nil
}

// EnsureDirs 创建所有尚不存在的目录及其父级，权限为 0700（仅所有者可读写执行）。
// 跨平台语义：非 Unix 系统上 0700 会被映射为合理的受限权限。
func (p Paths) EnsureDirs() error {
	mode := os.FileMode(0o700)
	for _, dir := range []string{p.ConfigDir, p.DataDir, p.CacheDir, p.LogDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, mode); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}
