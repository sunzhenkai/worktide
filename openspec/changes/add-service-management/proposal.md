## Why

WorkTide 是用户的终端工作流中心，但目前缺少统一管理「本地长驻开发进程」（如本地 API 服务器、构建守护进程、容器等）的手段。开发者通常要为这些进程维护多套脚本或 tmux 会话，缺乏一致的状态查询、日志查看与启停入口。本变更将服务管理纳入 WorkTide，使 TUI 与 CLI 共享同一份服务模型，从而把工作流中心从「工具聚合」升级为「工具 + 长驻进程聚合」。

## What Changes

- 新增 `worktide svc` 子命令（cobra），覆盖 run / list / status / logs / kill / restart / clean / rm / dir
- 新增 `internal/service` 包，统一管理服务声明与运行态（基于 bbolt 持久化，跨进程安全）
- 新增 `services` 工具，注册到 TUI 左侧导航，提供列表、日志、启停的交互式操作
- 扩展 `config.yaml`：新增 `services:` 段（声明式服务定义）与 `include:` 指令（深度合并外部 YAML 文件）
- 新增 `services.yaml`：CLI 写入的用户运行时声明，与 `config.yaml` 合并；同名条目以 `services.yaml` 为准并提示警告
- 声明与运行态严格分离：YAML 只存声明（name → cwd/command），运行态（PID、日志路径等）由 bbolt 维护

## Capabilities

### New Capabilities

- `service-management`：服务定义、生命周期管理、CLI/Tool 双入口、运行态持久化与日志查看
- `config-include`：`include:` 路径列表的解析、`~` 与 `${ENV}` 展开、deep merge 顺序与循环检测

### Modified Capabilities

无

## 非目标

- 不实现远程服务管理（部署、远程启停）
- 不实现健康检查、自动重启、依赖编排等高级能力（仅手动启停 + clean）
- 不引入项目级 scope 概念（服务全机器一张表，按 cwd 元数据分组显示）
- 不修改现有 tool-registry、local-backend、workspace-config 的对外行为（仅在配置层扩展读取逻辑）

## 影响面

- 新增：`internal/service/`、`internal/tools/builtin/services.go`、`cmd/worktide/{root,svc}.go`
- 修改：`internal/config/{config,loader,paths}.go`、`internal/tools/builtin/builtin.go`、`go.mod`（新增 spf13/cobra 依赖）
- 新文件：`$WORKTIDE_CONFIG_DIR/services.yaml`（CLI 首次写入时创建）

## 适用人群

- 在本地维护多个长驻开发进程的全栈 / 后端 / 平台工程师
- 希望在统一 TUI 中同时管理工具与服务、避免多窗口切换的用户
