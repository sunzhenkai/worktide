## Context

WorkTide 是一个全新空仓库，目标是构建一个 Go 编写的 TUI 个人中心，将分散的命令行工具聚合为一个可个性化、可离线运行的工作台。当前没有任何代码、依赖或目录约定，本设计从零规划项目骨架、技术栈与核心架构，作为后续所有工具模块接入的基础。

约束：
- 编程语言：Go（建议 1.22+，使用 `log/slog` 标准库）。
- 输出语言：简体中文（UI 文案、文档、注释）。
- 跨平台优先：macOS / Linux / Windows。
- 单机本地优先，本期不做云同步。
- 关键交互层保留可替换空间，避免与单一框架不可逆耦合。

## Goals / Non-Goals

**Goals:**
- 奠定清晰的 Go 工程目录与构建入口，便于长期演进。
- 提供一个能承载多工具的 TUI 主壳（布局、导航、快捷键、主题）。
- 建立可插拔的工具注册机制，新工具可自描述接入、按需启停。
- 统一配置与本地数据目录约定，跨平台透明。
- 提供与 TUI 解耦的可选本地后端，承担异步任务与持久化。
- 工程化基础（构建脚本、lint、CI、README、示例配置）。

**Non-Goals:**
- 不实现具体业务工具，仅交付框架与一个示例占位工具。
- 不提供云账号、多端同步、远程协作。
- 不做 GUI 与移动端。
- 不引入重型运行时依赖（如强制外部数据库服务）。

## Decisions

### 决策 1：TUI 框架选用 Bubble Tea（Elm 架构）

采用 `github.com/charmbracelet/bubbletea` + `bubbles` + `lipgloss`。
- **理由**：采用 The Elm Architecture（Model/Update/View），状态可预测、易测试；社区活跃、组件生态完整；纯 Go、跨平台、支持鼠标与终端宽度自适应；`lipgloss` 提供稳定样式抽象，便于主题化。
- **备选与取舍**：
  - `rivo/tview`：组件齐全、偏传统 Widget 模型，但状态管理较命令式、样式定制较繁琐，扩展大型应用时易产生状态耦合。
  - `gizak/termui`：偏仪表盘场景，交互能力弱，不适合工作台。
- **隔离**：在 `internal/ui` 内统一封装 `bubbletea` 调用，业务与工具层不直接依赖框架类型，便于未来替换。

### 决策 2：配置体系选用 Viper

采用 `github.com/spf13/viper`，支持 YAML/TOML/JSON、环境变量、默认值、热加载。
- **理由**：生态最成熟、文档丰富、多格式与多来源开箱即用，降低用户上手成本。
- **备选与取舍**：`knadh/koanf` 更轻量、无泛型历史包袱，但生态与示例相对较少；考虑到本期优先「主流稳健」，选择 Viper，后续若体积敏感可迁移。

### 决策 3：日志选用标准库 `log/slog`

- **理由**：Go 1.21+ 内置结构化日志，零外部依赖、API 稳定、性能足够，避免 `zap`/`zerolog` 的额外依赖与学习成本。
- **取舍**：放弃 `zap` 的极致性能，换取更少依赖与更好可移植性。

### 决策 4：跨平台目录用标准库 + 显式约定

使用 `os.UserConfigDir()` / `os.UserHomeDir()` / `os.UserCacheDir()`（1.22+）解析：
- 配置：`<UserConfigDir>/worktide/config.yaml`
- 数据：`<UserHomeDir>/.local/share/worktide`（Linux 约定）或平台等价目录
- 缓存：`<UserCacheDir>/worktide`
- **备选**：`adrg/xdg` 提供更完整 XDG 语义；本期先用标准库，必要时再引入。

### 决策 5：工具注册采用「自描述 + 显式注册」可插拔机制

定义 `tool.Tool` 接口（元信息 + 生命周期 + 渲染 + 事件处理），工具在 `init()` 或通过 `Register()` 注册到全局 `Registry`。主壳根据配置的「启用列表」决定加载哪些工具，并聚合到导航菜单。
- **理由**：显式注册比反射/插件 DLL 更简单可靠、易调试；Go 插件机制（`plugin` 包）跨平台支持差（尤其 Windows/macOS 限制），故不采用动态加载。
- **取舍**：放弃运行时热插拔，换取稳定与跨平台一致性；工具需在编译期纳入。

### 决策 6：本地后端为「同进程 + 接口抽象」，预留 IPC 演进

本期后端以同进程 goroutine 服务形式运行，通过 `backend.Service` 接口暴露任务队列、缓存与持久化能力。TUI 与后端通过 Go channel / 接口解耦，不直接共享状态。
- **理由**：同进程模型部署最简、无端口/进程管理负担；接口抽象使未来可平滑替换为独立进程（gRPC/HTTP）而不影响上层。
- **备选**：直接上 gRPC 独立进程——增加部署与调试复杂度，不符合「最小可用」原则。

### 决策 7：持久化分级（文件 + 嵌入式 KV）

- 配置/偏好：YAML 文件（人可读可编辑）。
- 结构化数据/缓存：`go.etcd.io/bbolt`（嵌入式 KV，纯 Go、ACID、无外部服务）。
- **理由**：分级匹配不同访问模式；bbolt 简单可靠，避免引入 SQLite 的 cgo 依赖（`modernc.org/sqlite` 纯 Go 但体积较大，本期暂不需要）。
- **取舍**：放弃 SQL 查询能力，换取零外部服务与轻量；如未来工具需要复杂查询再引入 SQLite。

## 模块划分与数据流

### 目录结构

```
worktide/
├── cmd/worktide/         # 程序入口 main.go
├── internal/
│   ├── app/              # 应用编排：依赖装配、生命周期、Bubble Tea 程序
│   ├── ui/               # UI 层：主题、布局、导航、全局快捷键（封装 bubbletea）
│   ├── tools/            # 工具注册中心 + Tool 接口 + 内置示例工具
│   ├── config/           # 配置加载、默认值、路径解析
│   └── backend/          # 本地后端：任务队列、缓存、bbolt 持久化
├── pkg/                  # （预留）对外 SDK，供外部工具实现 Tool 接口
├── configs/              # 示例配置文件
├── scripts/              # 构建/安装脚本
├── .github/workflows/    # CI
├── go.mod / go.sum
├── Makefile / Taskfile   # 构建任务
└── README.md
```

### 核心数据流

```
用户键鼠输入
     │
     ▼
Bubble Tea ──Msg──▶ app（Update）
     │                  │
     │                  ▼ （路由）
     │            tools.Registry ──分发──▶ 当前激活工具 (Tool)
     │                                  │
     │                                  ▼ （需要数据/异步）
     │                              backend.Service
     │                                  │
     │                                  ▼
     │                          bbolt / 配置文件 / 缓存
     │                                  │
     ▼ ◀──── 结果 Msg ◀────────────────┘
View 渲染（lipgloss 样式）
```

要点：
- 工具不直接访问持久化，统一经由 `backend.Service` 接口，保证可替换与可测试。
- UI 层屏蔽 `bubbletea` 细节，工具实现面向稳定抽象，降低框架锁定。
- 配置变更通过 Viper 热加载（或重启生效），后端与工具订阅所需键。

## Risks / Trade-offs

- **[风险] Bubble Tea 复杂界面的状态管理** → 用 Elm 架构严格分层、为每个工具隔离 model；提供基类组合工具减少样板。
- **[风险] 显式注册导致工具必须改主干源码** → 在 `internal/tools/builtin` 集中登记，文档化「五步接入」流程；中长期评估 WASM/进程插件。
- **[风险] Viper 依赖体积偏大** → 监控二进制体积；必要时迁移至 `koanf`（接口已抽象）。
- **[风险] 跨平台路径差异（尤其 Windows）** → 统一在 `internal/config` 封装并加单测覆盖三平台预期目录。
- **[风险] 同进程后端阻塞 UI** → 后端任务异步执行，结果经 channel 回传为 Msg，UI 永不阻塞。
- **[取舍] 放弃运行时热插拔** → 换取跨平台稳定；编译期集成工具。

## Migration Plan

全新项目，无迁移。落地顺序：
1. 初始化 `go.mod` 与目录骨架。
2. 配置 + 路径解析（含测试）。
3. UI 主壳 + 主题 + 导航。
4. 工具注册中心 + 示例工具。
5. 后端基础（bbolt + 任务队列）。
6. 工程化（Makefile/CI/lint/README）。

回滚策略：每阶段独立提交，任一阶段失败可回退至上一个可运行提交，不影响仓库可用性。

## Open Questions

- 是否需要在首期引入 `task`（Taskfile.yml）还是仅用 Makefile？倾向两者择一（默认 Makefile，Windows 提供 `go run` 脚本兜底）。
- 示例占位工具选什么（欢迎页 / 系统信息 / 便签）？建议「欢迎页 + 系统信息」最小集。
- 是否预留 i18n 框架（如 `golang.org/x/text/message`）？建议预留接口但本期仅简体中文。
