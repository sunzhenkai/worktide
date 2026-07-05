# WorkTide

> 你的个性化终端工作流中心。

WorkTide 是一个用 [Go](https://go.dev) 编写的 TUI（终端用户界面）个人中心，把日常分散的命令行工具聚合到一个可插拔、可个性化、可离线运行的工作台。每个人都可以在这里打造属于自己的工作流。

## 特性

- 🏠 **统一入口**：一个终端窗口承载所有工具，告别多窗口切换。
- 🧩 **可插拔工具**：工具以模块形式自描述注册，按需启用，接入简单。
- 🎨 **主题与快捷键**：内置默认/深色主题，全局快捷键可配置。
- 💾 **本地后端**：异步任务队列 + bbolt 键值持久化，单机离线优先。
- 🖥️ **跨平台**：支持 macOS / Linux / Windows，遵循各平台目录约定。
- 🇨🇳 **简体中文优先**：UI 文案与文档默认简体中文。

## 安装

要求 Go 1.22 及以上。

```bash
# 从源码安装到 $GOBIN
make install

# 或直接运行
make run

# 或构建到本地 bin/
make build
./bin/worktide
```

## 快速使用

```bash
worktide            # 启动主界面
worktide --help     # （后续版本提供）查看帮助
```

启动后：

- 使用 `↑/↓` 在左侧导航选择工具，`回车` 激活；
- 按 `Tab` 在导航与内容区之间切换焦点；
- 按 `1`–`9` 快速切换到对应工具；
- 按 `?` 打开帮助，按 `s` 打开设置，按 `q` 退出。

## 配置

配置文件位于各平台的标准配置目录：

| 平台    | 路径                                              |
|---------|---------------------------------------------------|
| macOS   | `~/Library/Application Support/worktide/config.yaml` |
| Linux   | `~/.config/worktide/config.yaml`（或 `$XDG_CONFIG_HOME`） |
| Windows | `%AppData%\worktide\config.yaml`                  |

完整示例见 [`configs/config.example.yaml`](configs/config.example.yaml)。关键项：

```yaml
theme:
  name: default        # default | dark
keymap:
  quit: "q"
  focus_nav: "tab"
  help: "?"
  settings: "s"
tools:
  enabled:             # 启用工具列表，为空时启用默认示例工具
    - welcome
    - sysinfo
backend:
  enabled: true        # 是否启用本地后端（关闭则降级运行）
```

配置优先级（高 → 低）：命令行标志 > 环境变量（`WORKTIDE_` 前缀）> 配置文件 > 内置默认值。主题与快捷键支持热加载，工具启用列表变更需重启。

## 目录结构

```
worktide/
├── cmd/worktide/         # 程序入口
├── internal/
│   ├── app/              # 应用编排：依赖装配与生命周期
│   ├── ui/               # TUI 主壳（封装 bubbletea）
│   │   └── theme/        # 主题
│   ├── tools/            # 工具注册中心与 Tool 接口
│   │   └── builtin/      # 内置示例工具（welcome / sysinfo）
│   ├── config/           # 配置加载与跨平台路径
│   └── backend/          # 本地后端：异步任务 + bbolt KV
├── pkg/                  # （预留）对外 SDK
├── configs/              # 示例配置
├── scripts/              # 构建/安装脚本
├── .github/workflows/    # CI
├── go.mod / go.sum
└── Makefile
```

## 开发

```bash
make help        # 查看所有可用目标
make test        # 运行测试
make vet         # 静态检查
make lint        # golangci-lint（需先安装）
make fmt         # 格式化
```

### 新增一个工具

1. 实现 `internal/tools.Tool` 接口（元信息 + `Init/Activate/Deactivate/Close` + `HandleKey` + `View`）；
2. 在 `internal/tools/builtin/builtin.go` 的 `RegisterAll` 中登记；
3. 如需默认启用，加入 `internal/config.Default().Tools.Enabled`。

> 工具需要持久化或异步任务时，统一通过 `backend.Service` 接口访问，不要直接读写文件。

### 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 风格，例如：

```
feat(tools): 新增便签工具
fix(config): 修复热加载空指针
docs(readme): 补充安装说明
```

## 架构概览

```
用户输入 → bubbletea → app(Update) → tools.Registry → 当前激活工具
                                  ↓ （需要数据/异步）
                              backend.Service → bbolt / 配置文件
                                  ↓
                               结果回传 → View 重绘
```

- **UI 隔离**：`internal/ui` 是唯一依赖 bubbletea 的层，工具仅依赖 `tools` 包的稳定抽象。
- **后端解耦**：TUI 与工具通过 `backend.Service` 接口访问后端，可平滑替换为进程间通信。
- **错误隔离**：单任务 panic 被捕获并转为错误结果，不影响整体应用。

## 许可证

[MIT](LICENSE)
