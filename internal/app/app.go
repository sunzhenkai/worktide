// Package app 负责 WorkTide 应用的依赖装配、生命周期编排与启动。
//
// 它将配置、UI、工具注册中心和本地后端串联为一个可运行的整体，
// 并统一处理 panic 恢复、错误打印与退出码。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sunzhenkai/worktide/internal/backend"
	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/tools"
	"github.com/sunzhenkai/worktide/internal/tools/builtin"
	"github.com/sunzhenkai/worktide/internal/ui"
)

// ExitCode 表示应用退出的语义化编码。
type ExitCode int

const (
	// ExitOK 表示正常退出。
	ExitOK ExitCode = 0
	// ExitErr 表示一般性错误。
	ExitErr ExitCode = 1
)

// Run 是应用的统一入口。它装配依赖、启动主循环，并在结束时返回退出码。
// 任何从依赖装配或主循环中逃逸的 panic 都会被恢复并转为非零退出码。
func Run(ctx context.Context, args []string) ExitCode {
	// 顶层 panic 防护：保证任何未捕获的异常都输出可读错误而非栈崩溃。
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "应用发生致命错误: %v\n", r)
			os.Exit(int(ExitErr))
		}
	}()

	// 1. 加载配置：缺失时使用默认值，不向用户报错。
	cfg, err := config.Load()
	if err != nil {
		// 配置加载出现非「文件不存在」类错误时，记录并继续使用默认值。
		// 此处尚未初始化结构化日志，使用 stderr 提示。
		fmt.Fprintf(os.Stderr, "配置加载警告: %v（将使用默认配置继续）\n", err)
	}

	// 2. 初始化结构化日志（输出到日志目录）。
	logger, logCloser, err := initLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		return ExitErr
	}
	defer logCloser()
	slog.SetDefault(logger)

	slog.Info("WorkTide 启动中", "version", Version, "go", GoVersion())

	// 3. 监听中断信号，支持优雅退出。
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 4. 装配并运行应用（后续阶段填充）。
	if err := runApplication(ctx, cfg); err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("收到退出信号，应用结束")
			return ExitOK
		}
		slog.Error("应用运行失败", "error", err)
		fmt.Fprintf(os.Stderr, "应用运行失败: %v\n", err)
		return ExitErr
	}

	return ExitOK
}

// runApplication 装配具体子系统并进入主循环：
// 解析目录 -> 装配后端 -> 注册工具 -> 应用启用列表 -> 启动 TUI -> 启动配置热加载。
func runApplication(ctx context.Context, cfg *config.Config) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return fmt.Errorf("解析目录失败: %w", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 1. 装配后端：按配置启用或降级。
	var be backend.Service
	if cfg.Backend.Enabled {
		svc, berr := backend.New(filepath.Join(paths.DataDir, "worktide.db"), slog.Default())
		if berr != nil {
			slog.Warn("后端初始化失败，降级为无后端模式", "error", berr)
			be = backend.NewDisabled()
		} else {
			be = svc
			defer func() { _ = svc.Close() }()
		}
	} else {
		be = backend.NewDisabled()
		slog.Info("后端已禁用，以无后端模式运行")
	}
	_ = be

	// 2. 注册中心与内置工具。
	registry := tools.NewRegistry()
	if err := builtin.RegisterAll(registry); err != nil {
		slog.Warn("部分内置工具注册失败", "error", err)
	}
	// 为 sysinfo 注入目录路径。
	if t, ok := lookupBuiltin(registry, "sysinfo"); ok {
		t.SetPaths(builtin.SysPaths{
			ConfigDir: paths.ConfigDir,
			DataDir:   paths.DataDir,
			CacheDir:  paths.CacheDir,
			LogDir:    paths.LogDir,
		})
	}

	// 3. 应用启用列表（config 已做未知 ID 过滤与空列表兜底）。
	enabled, err := registry.ApplyEnabled(ctx, cfg.Tools.Enabled)
	if err != nil {
		return fmt.Errorf("应用启用列表失败: %w", err)
	}
	slog.Info("已启用工具", "tools", enabled)

	// 4. 启动 TUI 程序。
	prog := ui.NewProgram(registry, cfg, nil)
	if err := prog.Run(ctx); err != nil {
		return fmt.Errorf("TUI 运行失败: %w", err)
	}

	// 5. 退出时释放工具资源。
	if err := registry.CloseAll(); err != nil {
		slog.Warn("释放工具资源时出错", "error", err)
	}
	return nil
}

// lookupBuiltin 在注册中心查找指定 ID 的工具。
func lookupBuiltin(registry *tools.Registry, id string) (*builtin.SysInfo, bool) {
	for _, t := range registry.EnabledTools() {
		if t.Meta().ID == id {
			if s, ok := t.(*builtin.SysInfo); ok {
				return s, true
			}
		}
	}
	return nil, false
}

// initLogger 的实现见 logger.go。

// Version 为当前应用版本，构建时可通过 -ldflags 注入。
var Version = "0.0.0-dev"
