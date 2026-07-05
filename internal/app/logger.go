package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sunzhenkai/worktide/internal/config"
)

// initLogger 初始化结构化日志，同时输出到日志文件与 stderr。
//
// 行为：
//   - 在日志目录下创建 worktide.log，以 append 方式写入；
//   - 创建失败时回退到仅 stderr，不阻止启动；
//   - 返回 closer 用于关闭日志文件句柄。
func initLogger(cfg *config.Config) (*slog.Logger, func(), error) {
	_ = cfg // 预留：后续可按 cfg 调整级别
	paths, err := config.ResolvePaths()
	if err != nil {
		return stderrLogger(), func() {}, nil
	}
	if err := paths.EnsureDirs(); err != nil {
		// 日志目录创建失败不阻止启动，回退 stderr。
		return stderrLogger(), func() {}, nil
	}
	logPath := filepath.Join(paths.LogDir, "worktide.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return stderrLogger(), func() {}, nil
	}
	w := io.MultiWriter(os.Stderr, f)
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	closer := func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "关闭日志文件失败: %v\n", cerr)
		}
	}
	return slog.New(handler), closer, nil
}

// stderrLogger 返回一个仅输出到 stderr 的简易日志器。
func stderrLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
