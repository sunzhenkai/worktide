## ADDED Requirements

### Requirement: 服务声明与合并

系统 SHALL 支持在 `config.yaml`（含 `include:` 引入的文件）和 `services.yaml` 中通过 `services:` 段声明本地服务，每条声明 MUST 包含 `cwd` 与 `command` 字段。`services.yaml` 与 `config.yaml` 的同名条目 SHALL 以 `services.yaml` 为准，并 MUST 输出一条警告。

#### Scenario: 仅在 config.yaml 声明

- **WHEN** `config.yaml` 含 `services.api = { cwd, command }`，`services.yaml` 不存在
- **THEN** 加载后内存中存在一条名为 `api` 的服务声明

#### Scenario: 仅在 services.yaml 声明

- **WHEN** `services.yaml` 含 `services.redis = { cwd, command }`，`config.yaml` 无相关条目
- **THEN** 加载后内存中存在一条名为 `redis` 的服务声明

#### Scenario: 同名条目以 services.yaml 为准

- **WHEN** `config.yaml` 含 `services.api = { cwd: A, command: make }`，`services.yaml` 含 `services.api = { cwd: B, command: pnpm dev }`
- **THEN** 生效的服务声明是 `services.yaml` 的版本（cwd=B, command=pnpm dev）
- **AND** stderr 或日志输出包含一条 WARN 级别消息指明服务名 `api`

#### Scenario: services.yaml 缺失

- **WHEN** `services.yaml` 不存在
- **THEN** 加载不报错，正常使用 config.yaml + include 的声明

### Requirement: 命令 shell 与 argv 双形态

服务声明中的 `command` 字段 MUST 接受 shell 字符串（如 `"make dev"`）或字符串数组（如 `["pnpm", "dev"]`）两种形态。

#### Scenario: shell 字符串形态

- **WHEN** `command: "make dev"`
- **THEN** 启动时通过 `sh -c "make dev"` 执行（Windows 通过 `cmd /C`）

#### Scenario: argv 数组形态

- **WHEN** `command: ["pnpm", "dev"]`
- **THEN** 启动时通过 `exec.Command("pnpm", "dev")` 执行

### Requirement: 命令行启动服务

`worktide svc run [name] [-- <command>]` SHALL 在后台启动服务并将其注册到运行态。无 name 时 MUST 使用当前目录的 basename 作为服务名。

#### Scenario: 启动已声明服务

- **WHEN** 用户执行 `worktide svc run api`，且 `api` 已存在于声明中
- **THEN** 服务按声明的 cwd 与 command 启动
- **AND** stdout 输出 `started service "api" (pid <N>)` 与日志文件路径
- **AND** 运行态记录新增一条 `Record{PID, PGID, Cwd, Command, LogPath, StartedAt}`

#### Scenario: 启动并指定命令

- **WHEN** 用户执行 `worktide svc run api -- pnpm dev`
- **THEN** 服务以 `pnpm dev` 启动
- **AND** `services.yaml` 中的 `api` 条目被更新为 `command: ["pnpm", "dev"]`

#### Scenario: 匿名启动

- **WHEN** 用户在 `/Users/me/work/api` 目录下执行 `worktide svc run -- make dev`
- **THEN** 服务名取 `api`（目录 basename）
- **AND** cwd 使用当前目录

#### Scenario: 同名已运行

- **WHEN** 用户执行 `worktide svc run api`，且 `api` 当前处于 running 状态
- **THEN** 命令以非零退出码退出
- **AND** stderr 包含「service "api" is already running」的提示

#### Scenario: --force 替换同名

- **WHEN** 用户执行 `worktide svc run --force api -- make dev`，且 `api` 当前处于 running 状态
- **THEN** 已运行的 `api` 进程被 SIGTERM 终止
- **AND** 新进程按 `make dev` 启动并登记

#### Scenario: --ephemeral 不写盘

- **WHEN** 用户执行 `worktide svc run --ephemeral -- make dev`
- **THEN** 服务正常启动并登记运行态
- **AND** `services.yaml` 不被创建或修改

### Requirement: 进程组隔离

服务进程 SHALL 在 Unix 系统上通过 `Setpgid: true` 自成进程组，记录其 PGID；`kill` 与 `kill -9` 命令 MUST 向 `-pgid` 发送信号而非仅 PID。

#### Scenario: 启动时 Setpgid

- **WHEN** 在 macOS/Linux 上启动服务
- **THEN** 服务进程的 PGID 等于其 PID
- **AND** 启动失败时退出码非零且 stderr 含原因

#### Scenario: kill 终止进程组

- **WHEN** 服务进程已 fork 出子进程，用户执行 `worktide svc kill api`
- **THEN** 服务进程及其衍生子进程全部收到 SIGTERM 并退出

#### Scenario: kill -9 强杀

- **WHEN** 用户执行 `worktide svc kill -9 api`
- **THEN** 服务进程组收到 SIGKILL 并被强制终止

### Requirement: 列出服务与状态

`worktide svc list` SHALL 列出所有服务的名称、状态、cwd；`-v` 时额外显示 command 与 log 路径。状态由当前 PID 是否存活决定：`running`（存活）/ `exited`（未存活且上次为 running）/ `stale`（其他）。

#### Scenario: 默认列表

- **WHEN** 用户执行 `worktide svc list`
- **THEN** stdout 表格列包含 `NAME / STATUS / PID / PATH`

#### Scenario: 详细列表

- **WHEN** 用户执行 `worktide svc list -v`
- **THEN** stdout 表格列包含 `NAME / STATUS / PID / PATH / COMMAND / LOG`

#### Scenario: 空列表

- **WHEN** 系统中无任何已注册服务
- **THEN** stdout 输出 `No services found.`，退出码为 0

#### Scenario: 单服务状态查询

- **WHEN** 用户执行 `worktide svc status api`
- **THEN** stdout 显示 `api` 的详细状态（`-v` 等价输出）

### Requirement: 查看与跟踪日志

`worktide svc logs <name>` SHALL 打印日志末尾若干行（默认 50）；`-n N` 指定行数；`-f` 进入 follow 模式持续输出；`--open` 用系统编辑器或 opener 打开日志文件。

#### Scenario: tail 默认

- **WHEN** 用户执行 `worktide svc logs api`
- **THEN** stdout 输出 `api.log` 的最后 50 行

#### Scenario: 指定行数

- **WHEN** 用户执行 `worktide svc logs -n 200 api`
- **THEN** stdout 输出 `api.log` 的最后 200 行

#### Scenario: follow 模式

- **WHEN** 用户执行 `worktide svc logs -f api`
- **THEN** 程序持续输出 `api.log` 的新增内容直至用户按 Ctrl+C

#### Scenario: 日志文件不存在

- **WHEN** 服务的日志文件尚未创建（启动失败或未启动）
- **THEN** 命令以非零退出码退出
- **AND** stderr 包含「log file not found: <path>」

### Requirement: 重启服务

`worktide svc restart <name>` SHALL 停止当前服务（如在运行）并重新按声明启动。

#### Scenario: 运行时重启

- **WHEN** `api` 处于 running，用户执行 `worktide svc restart api`
- **THEN** `api` 进程收到 SIGTERM
- **AND** 新进程按声明的 cwd 与 command 启动
- **AND** stdout 提示 `restarted service "api" (pid <N>)`

#### Scenario: 未运行时重启

- **WHEN** `api` 处于 exited/stale，用户执行 `worktide svc restart api`
- **THEN** 直接按声明启动新进程，不发信号

#### Scenario: 无可用声明重启

- **WHEN** `api` 不在 `services.yaml` 与 `config.yaml` 任一处声明中
- **THEN** 命令以非零退出码退出
- **AND** stderr 提示「service "api" has no command to restart」

### Requirement: 清理服务记录

`worktide svc clean` SHALL 移除所有 exited/stale 状态的运行态记录；`--logs` 同时删除对应日志文件；`--all` 同时移除状态为 running 的记录（用于彻底清空）。

#### Scenario: 默认清理

- **WHEN** 存在 exited/stale 记录若干
- **THEN** 这些记录从运行态存储中被移除
- **AND** stdout 输出 `cleaned <N> service record(s)`

#### Scenario: --logs 同时清日志

- **WHEN** 用户执行 `worktide svc clean --logs`
- **THEN** 所有被清理的记录对应的 `<name>.log` 文件一并删除

#### Scenario: --all 包含 running

- **WHEN** 用户执行 `worktide svc clean --all`
- **THEN** 所有记录被移除（包括 running 状态的）
- **AND** 运行中的进程不被信号终止（仅移除登记）

### Requirement: 删除声明

`worktide svc rm <name>` SHALL 从 `services.yaml` 移除指定 name 的声明条目；MUST 在 name 同时存在于 `config.yaml` 或 include 文件时拒绝并提示。

#### Scenario: 删除仅 services.yaml 来源

- **WHEN** `api` 仅存在于 `services.yaml`
- **THEN** 该条目从 `services.yaml` 移除
- **AND** 如果运行态有同名记录，进程不被终止（用户需手动 `svc kill`）

#### Scenario: 拒绝删除 config 来源

- **WHEN** `api` 来自 `config.yaml` 或 include 文件
- **THEN** 命令以非零退出码退出
- **AND** stderr 提示「service "api" 来源于 config.yaml，请到该文件删除」

#### Scenario: 不存在的服务名

- **WHEN** `api` 不存在于任何声明与运行态
- **THEN** 命令以非零退出码退出
- **AND** stderr 提示「service "api" not found」

### Requirement: 输出服务目录

`worktide svc dir <name>` SHALL 输出服务的当前 cwd，stdout 单行，便于 shell 函数跳转。

#### Scenario: 输出声明中的 cwd

- **WHEN** 用户执行 `worktide svc dir api`
- **THEN** stdout 单行输出 `api` 声明中的 cwd（绝对路径）
- **AND** 退出码为 0

### Requirement: 运行态持久化

服务运行态 MUST 持久化到 worktide 的 bbolt 数据库（bucket `services`）；CLI 与 TUI 共享同一份存储；进程崩溃或重启后记录仍在。

#### Scenario: 进程重启后记录保留

- **WHEN** 用户启动 `api` 后强制 kill `worktide` 进程
- **AND** 重新启动 `worktide` 并执行 `svc list`
- **THEN** `api` 记录仍在列表中，状态按 PID 是否存活评估

#### Scenario: 跨实例并发

- **WHEN** 同时启动两个 `worktide svc run` 进程
- **THEN** 两个进程通过 bbolt 事务串行化读写，不出现丢失更新

### Requirement: TUI 服务工具

`services` SHALL 作为内置 Tool 注册到 worktide 工具中心；`tools.enabled` 中加入 `services` 即在左侧导航显示。激活后 MUST 提供：服务列表、当前选中项状态、最后 N 行日志预览、启停 / 重启 / 清理 / 打开日志的快捷键。

#### Scenario: 注册与启用

- **WHEN** `config.yaml` 含 `tools.enabled: [services]`
- **THEN** TUI 启动后左侧导航含「服务」一栏

#### Scenario: 列表展示

- **WHEN** 系统中存在 N 条服务记录
- **THEN** 激活 services tool 后主区域左侧显示 N 行（NAME / STATUS）
- **AND** 当前选中项的状态与日志预览在右侧刷新

#### Scenario: 快捷键操作

- **WHEN** 在 services tool 中按下 `k`
- **THEN** 选中服务被发送 SIGTERM
- **AND** 列表状态在下一个轮询周期刷新为 exited

#### Scenario: 未启用时不渲染

- **WHEN** `tools.enabled` 不含 `services`
- **THEN** 左侧导航不显示「服务」一栏

### Requirement: CLI 入口分发

`worktide` 命令 SHALL 根据 `os.Args` 分发：无 cobra 子命令时进入 TUI（`app.Run`），有 cobra 子命令时执行对应命令并以对应退出码退出。

#### Scenario: 无参数启动 TUI

- **WHEN** 用户执行 `worktide`（无参数）
- **THEN** 应用进入全屏 TUI 主界面

#### Scenario: 带 svc 子命令

- **WHEN** 用户执行 `worktide svc list`
- **THEN** 不进入 TUI，直接执行 list 子命令
- **AND** 退出码反映命令执行结果

#### Scenario: --help 输出

- **WHEN** 用户执行 `worktide --help` 或 `worktide svc --help`
- **THEN** stdout 显示 cobra 自动生成的帮助文本
- **AND** 退出码为 0

#### Scenario: 未知子命令

- **WHEN** 用户执行 `worktide unknown-cmd`
- **THEN** cobra 输出「Error: unknown command "unknown-cmd"」并提示可用命令
- **AND** 退出码非零
