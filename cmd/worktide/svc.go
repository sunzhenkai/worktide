package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"

	"github.com/sunzhenkai/worktide/internal/app"
	"github.com/sunzhenkai/worktide/internal/config"
	"github.com/sunzhenkai/worktide/internal/service"
)

// svcRunner 把一次 svc 命令运行所需的依赖聚合在一起。
type svcRunner struct {
	mgr   *service.Manager
	cfg   *config.Config
	paths config.Paths
	db    *bolt.DB
}

// newSvcRunner 构造一个 svc 命令的运行器。
// 完成后调用 close() 释放资源。
func newSvcRunner(ctx context.Context) (*svcRunner, error) {
	res, err := config.LoadFull()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	paths := res.Paths
	if err := paths.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}
	dbPath := paths.DataFile()
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("打开服务数据库失败: %w", err)
	}
	mgr, err := service.NewManager(db, res.Config.Services, paths.DataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	_ = ctx
	return &svcRunner{
		mgr:   mgr,
		cfg:   res.Config,
		paths: paths,
		db:    db,
	}, nil
}

func (r *svcRunner) close() {
	if r.db != nil {
		_ = r.db.Close()
	}
}

func newSvcCmd() *cobra.Command {
	svc := &cobra.Command{
		Use:   "svc",
		Short: "管理本地长驻服务（run / list / status / logs / kill / restart / rm / clean / dir）",
	}
	svc.AddCommand(
		newSvcRunCmd(),
		newSvcListCmd(),
		newSvcStatusCmd(),
		newSvcLogsCmd(),
		newSvcKillCmd(),
		newSvcRestartCmd(),
		newSvcRmCmd(),
		newSvcCleanCmd(),
		newSvcDirCmd(),
	)
	return svc
}

// ---- run ----

func newSvcRunCmd() *cobra.Command {
	var force bool
	var ephemeral bool
	cmd := &cobra.Command{
		Use:                "run [name] -- <command>",
		Short:              "启动服务并登记运行态",
		DisableFlagParsing: true,
		Long: strings.Join([]string{
			"启动后台服务并将其注册到运行态。",
			"无 name 时使用当前目录的 basename。",
			"-- 后是命令; --force 替换同名; --ephemeral 不写 services.yaml。",
			"-- 后只跟一个参数视为 shell 命令,多个参数视为 argv。",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 手动解析已知 flag。
			rest := make([]string, 0, len(args))
			for _, a := range args {
				switch a {
				case "--force":
					force = true
				case "--ephemeral":
					ephemeral = true
				case "-h", "--help":
					return cmd.Help()
				default:
					rest = append(rest, a)
				}
			}
			name, command, err := parseRunArgs(rest)
			if err != nil {
				return err
			}
			if name == "" {
				cwd, _ := os.Getwd()
				name = filepath.Base(cwd)
			}
			if name == "" || name == "." || name == "/" {
				return errors.New("cannot determine service name (no basename in cwd), please provide name explicitly")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			cwd, _ := os.Getwd()
			opts := service.RunOptions{
				Name:    name,
				Cwd:     cwd,
				Command: command,
				Env:     r.cfg.Services[name].Env,
				Force:   force,
			}
			rec, err := r.mgr.Run(opts)
			if err != nil {
				return err
			}
			fmt.Printf("started service %q (pid %d)\n", rec.Name, rec.PID)
			fmt.Printf("log: %s\n", rec.LogPath)
			if !ephemeral {
				def := config.ServiceDef{
					Cwd:     cwd,
					Command: command,
				}
				if err := service.UpsertServiceDecl(r.paths.ServicesFile(), name, def); err != nil {
					slog.Warn("write services.yaml failed", "error", err)
				}
			}
			return nil
		},
	}
	return cmd
}

// parseRunArgs 解析 [name] 与 -- cmd...。
//   - 无 --：args[0] 视为 name，命令为空
//   - 有 --：-- 前第一个元素为 name（可为空），-- 后是命令
//   - 命令仅 1 个参数：shell 模式
//   - 命令 2+ 个参数：argv 模式
func parseRunArgs(args []string) (string, service.ServiceCommand, error) {
	idx := -1
	for i, a := range args {
		if a == "--" {
			idx = i
			break
		}
	}
	var name string
	if idx == -1 {
		if len(args) > 0 {
			name = args[0]
		}
		return name, service.ServiceCommand{}, nil
	}
	if idx > 0 {
		name = args[0]
	}
	rest := args[idx+1:]
	if len(rest) == 0 {
		return name, service.ServiceCommand{}, errors.New("no command after --")
	}
	if len(rest) == 1 {
		return name, service.ServiceCommand{Shell: true, Cmd: []string{rest[0]}}, nil
	}
	return name, service.ServiceCommand{Shell: false, Cmd: rest}, nil
}

// ---- list ----

func newSvcListCmd() *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all services",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			list, err := r.mgr.List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Println("No services found.")
				return nil
			}
			headers := []string{"NAME", "STATUS", "PID", "PATH"}
			if verbose {
				headers = append(headers, "COMMAND", "LOG")
			}
			fmt.Println(strings.Join(headers, "\t"))
			for _, e := range list {
				pid := "-"
				path := "-"
				if e.Decl != nil {
					path = e.Decl.Cwd
				}
				if e.Record != nil {
					pid = fmt.Sprint(e.Record.PID)
				}
				row := []string{e.Name, string(e.Status), pid, path}
				if verbose && e.Record != nil {
					row = append(row, e.Record.Command.String(), e.Record.LogPath)
				} else if verbose {
					row = append(row, "-", "-")
				}
				fmt.Println(strings.Join(row, "\t"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show command and log path")
	return cmd
}

// ---- status ----

func newSvcStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Show service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("status requires name argument")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			for _, name := range args {
				e, err := r.mgr.Status(name)
				if err != nil {
					return err
				}
				fmt.Printf("name:    %s\n", e.Name)
				fmt.Printf("status:  %s\n", e.Status)
				if e.Record != nil {
					fmt.Printf("pid:     %d\n", e.Record.PID)
					fmt.Printf("started: %s\n", e.Record.StartedAt.Format(time.RFC3339))
					fmt.Printf("cwd:     %s\n", e.Record.Cwd)
					fmt.Printf("command: %s\n", e.Record.Command.String())
					fmt.Printf("log:     %s\n", e.Record.LogPath)
				} else {
					fmt.Println("pid:     -")
					fmt.Println("started: -")
				}
				fmt.Println()
			}
			return nil
		},
	}
}

// ---- logs ----

func newSvcLogsCmd() *cobra.Command {
	var n int
	var follow bool
	var open bool
	cmd := &cobra.Command{
		Use:   "logs [name]",
		Short: "View service logs (-n lines / -f follow / --open)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("logs requires name argument")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			name := args[0]
			e, err := r.mgr.Status(name)
			if err != nil {
				return err
			}
			logPath := r.mgr.LogPath(name)
			if e.Record != nil {
				logPath = e.Record.LogPath
			}
			if open {
				return service.OpenLog(logPath)
			}
			if n > 0 {
				lines, err := service.ReadTailLines(logPath, n)
				if err != nil {
					return err
				}
				for _, l := range lines {
					fmt.Println(l)
				}
				if !follow {
					return nil
				}
			}
			return service.FollowLog(cmd.Context(), logPath, os.Stdout)
		},
	}
	cmd.Flags().IntVarP(&n, "lines", "n", 30, "tail line count")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().BoolVar(&open, "open", false, "open log file with system default program")
	return cmd
}

// ---- kill ----

func newSvcKillCmd() *cobra.Command {
	var sig9 bool
	cmd := &cobra.Command{
		Use:   "kill [name]",
		Short: "Stop service (-9 sends SIGKILL)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("kill requires name argument")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			for _, name := range args {
				if err := r.mgr.Kill(name, sig9); err != nil {
					return err
				}
				fmt.Printf("killed %q\n", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&sig9, "nine", "9", false, "use SIGKILL")
	return cmd
}

// ---- restart ----

func newSvcRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [name]",
		Short: "Restart service using its declaration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("restart requires name argument")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			for _, name := range args {
				rec, err := r.mgr.Restart(name)
				if err != nil {
					return err
				}
				fmt.Printf("restarted %q (pid %d)\n", rec.Name, rec.PID)
			}
			return nil
		},
	}
}

// ---- rm ----

func newSvcRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm [name]",
		Short: "Remove service declaration from services.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("rm requires name argument")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			for _, name := range args {
				decls, _ := service.LoadServicesDecls(r.paths.ServicesFile())
				if _, ok := decls[name]; !ok {
					return fmt.Errorf("service %q not found", name)
				}
				if err := service.RemoveServiceDecl(r.paths.ServicesFile(), name); err != nil {
					return err
				}
				if err := r.mgr.RemoveDecl(name); err != nil {
					slog.Warn("remove decl failed", "error", err)
				}
				fmt.Printf("removed %q from services.yaml\n", name)
			}
			return nil
		},
	}
}

// ---- clean ----

func newSvcCleanCmd() *cobra.Command {
	var logs bool
	var all bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean exited/stale records (--logs also removes log files; --all includes running)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			n, err := r.mgr.Clean(service.CleanOptions{Logs: logs, All: all})
			if err != nil {
				return err
			}
			fmt.Printf("cleaned %d service record(s)\n", n)
			return nil
		},
	}
	cmd.Flags().BoolVar(&logs, "logs", false, "also remove log files")
	cmd.Flags().BoolVar(&all, "all", false, "include running records")
	return cmd
}

// ---- dir ----

func newSvcDirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dir [name]",
		Short: "Print service working directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("dir requires name argument")
			}
			r, err := newSvcRunner(cmd.Context())
			if err != nil {
				return err
			}
			defer r.close()
			e, err := r.mgr.Status(args[0])
			if err != nil {
				return err
			}
			cwd := ""
			if e.Decl != nil {
				cwd = e.Decl.Cwd
			} else if e.Record != nil {
				cwd = e.Record.Cwd
			}
			if cwd == "" {
				return fmt.Errorf("service %q has no cwd", args[0])
			}
			fmt.Println(cwd)
			return nil
		},
	}
}

// _ = app.Version 在编译期保证 app 包被引用。
var _ = app.Version
