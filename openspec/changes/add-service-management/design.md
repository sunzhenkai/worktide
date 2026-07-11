## Context

WorkTide 当前是 TUI 个人中心（cmd/worktide → internal/app → ui + tools + backend），具备：

- viper 配置加载（`internal/config`），目录解析遵循 macOS/Linux/Windows 平台约定
- bbolt 后端（`internal/backend`），支持 KV 与异步任务
- `Tool` 注册中心（`internal/tools`），欢迎页与 sysinfo 已注册
- fsnotify 热加载 watcher（仅主题/快捷键热生效，工具列表需重启）
- 零子命令、零服务管理能力

参考实现 grepom 的 `service` 子系统提供了成熟的本地服务管理语义（声明 + 运行态分离、`Setpgid` 进程组、tail/follow 日志、scope 隔离）。本变更在不引入项目级 scope 概念的前提下，重写一份 `internal/service` 并与 WorkTide 现有 viper + bbolt 后端对齐。

## Goals / Non-Goals

**Goals：**
- 提供统一的本地服务管理能力（CLI + TUI 双入口）
- 配置层支持 `include:` 指令，使配置可分项目组合（deep merge）
- 声明与运行态严格分离：YAML 只存声明、bbolt 只存运行态
- 与现有 `Tool` 接口对齐，service tool 可通过 `tools.enabled` 启用
- 复用现有 bbolt 后端与 fsnotify watcher，不引入新的并发原语

**Non-Goals：**
- 远程服务管理、健康检查、自动重启、依赖编排
- 项目级 scope（同一份 config 全机器共享一张表）
- 修改 `workspace-config` spec 已声明的行为（仅扩展读取能力）

## 模块划分

```
cmd/worktide/
├── main.go              ← 入口：解析 os.Args
│                          有 args → cli.Execute
│                          无 args → app.Run（TUI）
├── root.go              ← cobra root（无 args 时触发 TUI 入口）
└── svc.go               ← svc 子命令树

internal/config/
├── config.go            ← Config 结构追加 Services/Include 字段
├── loader.go            ← loadConfig：defaults → config.yaml → include loop → services.yaml
├── include.go           ← 新：路径解析（~/ / ${ENV}）、deep merge、循环检测
├── paths.go             ← 追加 ServicesFile() / DataFile()
└── *_test.go

internal/service/        ← 新包，照 grepom/service 重写
├── types.go             ← ServiceDef / ServiceCommand / Record / Status
├── manager.go           ← Manager.Run/List/Status/Logs/Kill/Restart/Clean/Remove
├── registry.go          ← bbolt-backed Registry（KV bucket "services"）
├── process.go           ← startCommand / Setpgid / killProcess
├── process_unix.go
├── process_windows.go
├── logs.go              ← ReadTailLines / ReadLinesFromOffset / FollowLog / OpenLog
└── *_test.go

internal/tools/builtin/
├── builtin.go           ← RegisterAll 新增 services
├── services.go          ← 新：services Tool
└── services_test.go
```

### 数据流

```
CLI 路径（worktide svc ...）
──────────────────────────────
os.Args → main.go
  ├─ 无 args / 无 cobra 子命令 → app.Run（进 TUI）
  └─ 有 cobra 子命令 → cli.Execute
        └─ svc.go 解析 flags
              ├─ loadServices() ← internal/config（合并 services.yaml + config.yaml）
              └─ manager.Run/Status/Kill/...
                    ├─ 读 / 写 bbolt bucket "services"
                    └─ 写日志到 $WORKTIDE_DATA_DIR/services/logs/<name>.log

TUI 路径（worktide → 选中「服务」工具）
──────────────────────────────────────
app.Run → ui.NewProgram
  └─ 选中 services tool → Activate
        ├─ 加载 Manager 实例（复用 CLI 的 Manager 构造）
        ├─ 启动轮询 goroutine：定期 List() 刷新状态
        └─ HandleKey：r/k/s/x → Manager.Restart/Kill/Run/Clean

配置加载（loadConfig）
─────────────────────
defaults → config.yaml → include[0..N] → services.yaml
                                                    ↑
                                         仅含 services: 段，CLI 写盘
```

## Decisions

### 1. CLI 框架：spf13/cobra + 子命令

- **理由**：grepom 已用 cobra，开发者熟悉；worktide 即将引入多个子命令；cobra 与 viper 同生态（spf13），集成顺畅。
- **备选**：
  - 自实现 flag 解析：节省依赖但重复造轮子，不值得
  - urfave/cli：API 类似 cobra 但生态弱、文档少
- **结论**：cobra。`go.mod` 新增 `github.com/spf13/cobra` 依赖。

### 2. 服务作为一等 Tool，注册到 `tools.enabled`

- **理由**：worktide 的核心定位是「工具聚合」，服务理应进入左侧导航，与 builtin 工具同层。
- **备选**：
  - 仅 CLI 可用、TUI 内不可见：简单但失去 TUI 一致性
  - 独立 panel（侧边常驻）：与现有「导航 + 内容」二分结构冲突
- **结论**：作为 builtin Tool 注册，`Meta.ID = "services"`、`Icon = "⚙"`；`tools.enabled` 中加入 `services` 即启用。

### 3. 全局一张表（无 scope）

- **理由**：worktide 是个人工作流中心；用户希望「所有服务都在这里」而非「按项目分散」。
- **备选**：
  - 按 cwd 自动分组（grepom 风格）：复杂度高、与单全局 config 不一致
  - 按声明来源分（config vs services）：体验割裂
- **结论**：bbolt 单 bucket `services`，所有 name 全局唯一；列表展示按 cwd 元数据分组（视觉分组，不影响存储）。
- **影响**：状态目录简化为 `$WORKTIDE_DATA_DIR/services/{logs/, worktide.db}`。

### 4. services.yaml 仅存声明

- **理由**：声明与运行态分离是干净的边界；YAML 可读可改、bbolt 适合频繁写；CLI 写盘范围最小化。
- **结构**：
  ```yaml
  # ~/.config/worktide/services.yaml
  services:
    api:
      cwd: /current/dir
      command: make dev
    redis:
      cwd: /usr/local/redis
      command: redis-server --port 6379
  ```
- **写盘约束**：
  - `worktide svc run ...` → upsert 一条声明
  - `worktide svc rm <name>` → 仅当该 name 来源于 services.yaml 时允许删除（防止误删 config.yaml/include 声明）
  - 不写 PID / log_path / started_at 等运行态字段

### 5. include 语义：list of paths → deep merge

- **理由**：用户希望将不同项目的服务声明外置到项目级 `.worktide.yaml`，主 config 通过 `include:` 引用；deep merge 与 viper `MergeConfig` 语义一致。
- **合并顺序**（后者覆盖前者）：
  1. 内置默认值
  2. `config.yaml` 顶层
  3. `config.yaml` 中 `include:` 列表按顺序合并
  4. `services.yaml`（最终覆盖 `services:` 段）
- **路径处理**：
  - `~` 展开为 `os.UserHomeDir()`
  - `${ENV_VAR}` 在合并前展开（unset 时报错并中止加载）
  - 不存在或非普通文件 → 报错并中止加载
- **循环检测**：visited-set（绝对路径）防止 a→b→a；超过 8 层深度即报错。
- **合并规则**：
  - map：递归逐 key 合并（后者覆盖前者）
  - list：后者完全替换前者（不追加）
  - 标量：后者覆盖前者
- **热加载**：include 路径的文件变更触发 fsnotify 事件 → Config 重载 → services map 更新；**已在跑的进程不受影响**（仅 UI 刷新）。如有新增/删除条目，TUI 顶部提示「配置已变更」。

### 6. 同名冲突策略：services.yaml 静默覆盖 + warn

- **理由**：CLI 是「我现在想这样」，比静态 config 更新更近；warn 即可避免静默破坏。
- **行为**：
  - `worktide svc run api -- pnpm dev` 写入 services.yaml 后，`api` 同时存在于 config.yaml
  - 加载时记录一行 `slog.Warn("服务名冲突，services.yaml 覆盖 config.yaml", "name", "api")`
  - 实际生效的是 services.yaml 的版本
- **备选**：
  - 报错拒绝：用户每次都得先 `rm` 再 `run`，体验差
  - 命名空间前缀（services.cli.api）：语义复杂、CLI 输出变长

### 7. 运行态存储：复用 bbolt

- **理由**：worktide 已有 bbolt 后端（`internal/backend`），KV 写并发安全；新增 JSON 文件 + flock 反而双系统。
- **schema**（bucket `services`）：
  - key：服务名（string）
  - value：`Record{Name, PID, PGID, Cwd, Command, CommandArgs, LogPath, StartedAt, LastStatus, ExitStatus}` JSON
- **事务**：所有读写经 `db.Update` / `db.View`；管理器持有 `*bolt.DB` 引用，由 app.Run 启动时构造一次。
- **与 backend.Service 接口的关系**：`internal/service` 不依赖 `backend.Service`（避免循环依赖）；直接接受 `*bolt.DB` 句柄。

### 8. 进程模型：Setpgid + 信号发给 `-pgid`

- **理由**：子进程自成进程组，子进程组 fork 的孙进程也能被一起 kill，避免「主进程死了孙子还在」。
- **实现**：`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`；记录 PGID；`kill` 时 `syscall.Kill(-pgid, sig)`。
- **Windows 兼容**：Windows 无 Setpgid；回退到 Job Object 或仅 PID 信号（实现简化）。

### 9. CLI 子命令清单

| 子命令 | 用途 | 关键 flag |
|---|---|---|
| `svc run [name] [-- cmd]` | 启动后台服务并登记 | `--force`、`--ephemeral`（不写盘） |
| `svc list` | 列出所有服务及状态 | `-v`（显示 command/log） |
| `svc status [name]` | 单服务状态详情 | — |
| `svc logs [name]` | 查看日志 | `-n` 行数、`-f` 跟随、`--open` 系统打开 |
| `svc kill [name]` | 停止服务 | `-9` SIGKILL |
| `svc restart [name]` | 重启服务 | — |
| `svc rm [name]` | 删除声明（仅 services.yaml 来源） | — |
| `svc clean` | 清理已退出/陈旧记录 | `--logs`、`--all` |
| `svc dir [name]` | 输出服务工作目录（供 shell 函数使用） | — |

### 10. TUI 内 services tool 形态

```
┌────────────┬────────────────────────────────────────┐
│ api   ●run │ > tail -n 30 of api.log                │
│ web   ○stop│ ...                                    │
│ redis ×err │                                        │
│ mongo ○off │ [r] restart [k] kill [s] start [l] log│
│            │ [x] clean  [?] help                    │
└────────────┴────────────────────────────────────────┘
  ↑ list        ↑ preview / actions
```

- 双面板：左 list、右 preview + 动作提示
- 状态轮询：1 Hz 调 `Manager.List()` 刷新状态
- 日志查看：复用 `service.ReadTailLines` + `service.FollowLog`
- 与 grepom `svc tui` 行为一致，但内嵌为 Tool 而非独立命令

## Risks / Trade-offs

- **bbolt 性能**：单文件 KV 在几十条服务规模下完全够用；上限 ~1000 条仍是 O(1)。如未来需高并发查询，可加内存索引。
- **services.yaml 写盘冲突**：并发多个 `svc run` 同时写盘 → 用临时文件 + `os.Rename` 原子替换；写盘前 flock（路径 `.lock`，与 grepom 类似）防止交错。
- **fsnotify 事件风暴**：编辑器保存 include 文件可能触发多次事件；viper 已做 100ms debounce，无需额外处理。
- **`~` 与 `${ENV}` 展开时机**：在合并前展开，错误立即失败；用户改 env 后需重启才能生效（不热生效 env 路径）。
- **TUI services tool 复杂度**：内嵌一个相对完整的服务面板会让该 Tool 偏离 worktide 其他 Tool 的「状态式」抽象；通过限定该 Tool 不参与通用热加载、单独维护 UI 模型降低耦合。
- **cobra 增加二进制大小**：约 +1.5 MB（cobra + pflag + 反射）；可接受。

## Migration Plan

无破坏性变更。引入顺序：

1. 先实现 `internal/config` 的 `include` 加载（不引入新命令）
2. 再实现 `internal/service` 包（不含 CLI）
3. 再接入 `cmd/worktide` 的 cobra root + svc 子命令（CLI 可用）
4. 最后注册 `services` builtin Tool（TUI 可用）
5. 灰度：services tool 默认不在 `tools.enabled` 中，需用户显式添加
6. 首次启动自动写入 `services.yaml` 不发生（仅 CLI 写入时创建）

回滚：删除 `internal/service/`、`cmd/worktide/{root,svc}.go`、`internal/tools/builtin/services.go`；用户配置 `services.yaml` 保留无害。

## Open Questions

1. **`worktide svc run --ephemeral` 是否真的需要？** 当前 PRD 未提；可能 v1 不做。
2. **TUI 启动时是否提示「检测到 N 个未运行服务，是否启动」？** 推荐 v1 不做，保持启动快速。
3. **服务与 bbolt 后端的 `Service` 接口命名冲突**：`internal/service.Manager` vs `internal/backend.Service`。考虑将新包命名为 `internal/servicesvc` 或 `internal/lifecycle`？倾向保留 `service`（grepom 同名；用户心智一致），在 backend 端加注释。
4. **`services.yaml` 是否允许包含 `include:` 指令？** 推荐允许（一致性），但实现成本 +10%。