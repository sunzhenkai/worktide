## 1. 初始化与依赖

- [x] 1.1 在 `go.mod` 中添加 `github.com/spf13/cobra` 依赖并运行 `go mod tidy`
- [x] 1.2 在 `internal/config/paths.go` 中新增 `ServicesFile()` 与 `DataFile()` 路径解析函数
- [x] 1.3 创建 `internal/service/` 包目录骨架与占位 `doc.go`
- [x] 1.4 在 `cmd/worktide/` 下创建 `root.go` 占位（cobra root，无子命令）

## 2. 配置层：include 与 services 段

- [x] 2.1 在 `internal/config/config.go` 的 `Config` 结构体中追加 `Services map[string]ServiceDef` 与 `Include []string` 字段
- [x] 2.2 在 `internal/config/config.go` 中新增 `ServiceDef` 与 `ServiceCommand` 类型（含 shell/argv 双形态 UnmarshalYAML）
- [x] 2.3 在 `internal/config/include.go` 中实现 `expandPath(s string) (string, error)`：处理 `~` 与 `${VAR}` / `${VAR:-default}` 展开
- [x] 2.4 在 `internal/config/include.go` 中实现 `deepMerge(dst, src map[string]any) error`：map 逐键合并，list 后者替换，类型冲突报错
- [x] 2.5 在 `internal/config/include.go` 中实现 `mergeIncludes(rootPath string, paths []string, visited map[string]bool, depth int) (map[string]any, error)`：循环检测 + 8 层深度上限
- [x] 2.6 在 `internal/config/loader.go` 的 `loadConfig` 中改造合并流程：defaults → config.yaml → include loop → services.yaml
- [x] 2.7 在 `internal/config/include_test.go` 中添加 `~` / `${ENV}` / 类型冲突 / 循环 / 深度超限 / 同文件多次出现的单元测试

## 3. service 包：核心类型与运行态

- [x] 3.1 在 `internal/service/types.go` 中定义 `Record`、`Entry`、`Status` 常量（`StatusRunning` / `StatusExited` / `StatusStale`）与 `Registry` 结构
- [x] 3.2 在 `internal/service/registry.go` 中实现 bbolt-backed `Open` / `Close` / `Get` / `Upsert` / `Delete` / `List` 接口
- [x] 3.3 在 `internal/service/process.go` 中实现 `startCommand(cwd, command, logPath)`：写日志分隔行、构造 `exec.Cmd`、绑定 stdout/stderr 到日志文件
- [x] 3.4 在 `internal/service/process_unix.go` 中实现 `isProcessAlive(pid)` 与 `signalProcess(pid, pgid, sig)`（使用 `syscall.Kill(-pgid, sig)`）
- [x] 3.5 在 `internal/service/process_windows.go` 中实现 Windows 等价（无 Setpgid，回退到 PID 信号）
- [x] 3.6 在 `internal/service/logs.go` 中实现 `ReadTailLines(path, n)`、`ReadLinesFromOffset(path, offset)`、`FollowLog(ctx, path, w)`、`OpenLog(path)`
- [x] 3.7 在 `internal/service/manager.go` 中实现 `Manager` 结构与构造函数 `NewManager(boltDB, decls map[string]ServiceDef, dataDir string)`
- [x] 3.8 在 `internal/service/manager.go` 中实现 `Run(opts)`：检查同名 → 启动进程 → 写分隔行 → 事务内 Upsert Record
- [x] 3.9 在 `internal/service/manager.go` 中实现 `Status(name)`、`List()`、`Kill(name, force)`、`Restart(name)`、`Clean(opts)`、`RemoveDecl(name)`（仅删声明）
- [x] 3.10 在 `internal/service/manager_test.go` 中添加运行态 + 进程生命周期的单元测试（用 sleep 子命令验证 PID 记录与 kill）

## 4. CLI 子命令

- [x] 4.1 在 `cmd/worktide/root.go` 中实现 cobra root：无子命令时调用 `app.Run`，有子命令时执行对应 cobra 命令
- [x] 4.2 在 `cmd/worktide/main.go` 中改造入口：使用 `cli.Execute()` 替换 `app.Run` 直接调用
- [x] 4.3 在 `cmd/worktide/svc.go` 中实现 `svc` 父命令与 9 个子命令骨架（run / list / status / logs / kill / restart / rm / clean / dir）
- [x] 4.4 在 `cmd/worktide/svc.go` 中实现 `svc run [name] [-- cmd]`：解析 name 与 `--` 分隔、构造 `RunOptions`、调用 `Manager.Run`、写 services.yaml
- [x] 4.5 在 `cmd/worktide/svc.go` 中实现 `svc list [-v]`、`svc status [name]`、`svc logs [-n] [-f] [--open] <name>`、`svc kill [-9] <name>`、`svc restart <name>`、`svc rm <name>`、`svc clean [--logs] [--all]`、`svc dir <name>`
- [x] 4.6 在 `internal/service/writer.go` 中实现 `UpsertServiceDecl(name, def)` 与 `RemoveServiceDecl(name)`：使用临时文件 + `os.Rename` 原子替换 services.yaml
- [x] 4.7 在 `cmd/worktide/svc_test.go` 中添加 cobra 子命令解析 + services.yaml 写盘的集成测试

## 5. TUI 服务工具

- [ ] 5.1 在 `internal/tools/builtin/services.go` 中实现 `Services` Tool：`Meta{ID: "services", Name: "服务", Icon: "⚙"}` 与生命周期方法
- [ ] 5.2 在 `internal/tools/builtin/services.go` 中实现双面板 model：左 list、右 preview（tail 30 行）+ 动作提示
- [ ] 5.3 在 `internal/tools/builtin/services.go` 中实现状态轮询（1 Hz 调 `Manager.List()`）与日志预览刷新
- [ ] 5.4 在 `internal/tools/builtin/services.go` 中绑定快捷键：`r` restart / `k` kill / `s` start / `l` 切换日志跟随 / `x` clean / `?` 帮助
- [ ] 5.5 在 `internal/tools/builtin/builtin.go` 的 `RegisterAll` 中追加 `Register(registry, builtin.NewServices(manager))`
- [ ] 5.6 在 `internal/tools/builtin/services_test.go` 中添加 model 初始化、状态刷新、按键路由的单元测试

## 6. 集成与打磨

- [ ] 6.1 在 `internal/config/loader.go` 中确保 fsnotify watcher 同时监听 `config.yaml`、`include` 列表中的文件与 `services.yaml`
- [ ] 6.2 在 `internal/service/manager.go` 中暴露 `Decls()` 与 `UpdateDecl(name, def)`，供配置热加载刷新视图
- [ ] 6.3 在 `internal/app/app.go` 中装配 `service.Manager`：在 backend 初始化后创建 Manager 并注入到 builtin.RegisterAll
- [ ] 6.4 在 `configs/config.example.yaml` 中追加 `include:` 与 `services:` 示例段
- [ ] 6.5 在 `README.md` 中新增「服务管理」章节：CLI 用法、include 写法、services.yaml 自动管理说明
- [ ] 6.6 在 `go test ./...` 中确认全部单测通过，并补充跨进程并发的集成测试（两个 worktide 实例同时 run / list）

## 7. 文档与发布

- [ ] 7.1 运行 `openspec validate add-service-management --strict` 校验所有 artifact 一致性
- [ ] 7.2 在 `internal/service/doc.go` 中补充包级中文文档注释
- [ ] 7.3 手动验证：执行 `worktide svc run -- make dev`，观察进程登记、日志写入、list 输出；执行 `worktide svc kill api` 验证进程组终止
- [ ] 7.4 手动验证：在 `config.yaml` 中添加 `include:`，观察配置合并与 fsnotify 热加载
