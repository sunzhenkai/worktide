## 1. 工程初始化

- [x] 1.1 初始化 `go.mod`，声明模块名 `github.com/sunzhenkai/worktide` 并设定 Go 版本下限（1.22）
- [x] 1.2 创建标准目录骨架：`cmd/worktide`、`internal/{app,ui,tools,config,backend}`、`pkg`、`configs`、`scripts`
- [x] 1.3 编写 `cmd/worktide/main.go` 最小入口，调用 `app.Run()` 并返回退出码
- [x] 1.4 在 `internal/app` 实现 `Run`：装配依赖、捕获 panic、统一错误打印与退出码
- [x] 1.5 验证 `go build ./...` 与 `go run ./cmd/worktide` 可成功执行（最小空壳不崩）

## 2. 配置与路径体系

- [x] 2.1 在 `internal/config` 封装跨平台目录解析（配置/数据/缓存/日志目录）并提供单测覆盖 macOS/Linux/Windows
- [x] 2.2 实现目录缺失时以权限 0700 自动创建（含父级）的逻辑
- [x] 2.3 引入 `spf13/viper`，实现默认值 + 配置文件 + 环境变量（`WORKTIDE_` 前缀）+ 命令行标志的合并加载
- [x] 2.4 定义配置结构体（含 `theme`、`keymap`、`tools.enabled`、`backend.enabled` 等字段）与默认值
- [x] 2.5 编写示例配置 `configs/config.example.yaml` 并由代码在配置缺失时使用默认值启动
- [x] 2.6 实现配置变更检测与热加载（主题/快捷键即时生效），不可热加载项提示重启
- [x] 2.7 补充 `internal/config` 单元测试（合并优先级、未知工具 ID 告警、空启用列表兜底）

## 3. UI 主壳与主题

- [x] 3.1 在 `internal/ui` 封装 `bubbletea` 程序创建与生命周期，业务层不直接依赖框架类型
- [x] 3.2 实现主布局：主内容区 + 常驻侧边导航区，支持焦点切换
- [x] 3.3 实现侧边导航：列出已启用工具、高亮当前激活项、键盘（方向键+回车）选择
- [x] 3.4 实现数字快捷键切换工具（按工具顺序绑定）
- [x] 3.5 定义全局快捷键集（退出/切焦点/帮助/设置）并支持从配置覆盖默认绑定
- [x] 3.6 实现帮助面板（`?` 触发），展示当前上下文快捷键
- [x] 3.7 实现主题模块（默认主题 + 至少一套备选），通过 `lipgloss` 统一样式
- [x] 3.8 实现终端尺寸自适应，窄屏降级为可滚动模式而非崩溃
- [x] 3.9 接入配置热加载，主题变更即时生效

## 4. 工具注册中心

- [x] 4.1 定义 `tool.Tool` 接口（元信息 + Init/Activate/Deactivate/Close + 事件处理 + 渲染）
- [x] 4.2 定义元信息结构（ID/显示名/描述/图标/版本），ID 约定 kebab-case
- [x] 4.3 实现 `Registry`：`Register`、查询、按启用列表过滤；重复 ID 注册返回错误
- [x] 4.4 实现工具生命周期调度：同一时刻仅一个激活，切换先 Deactivate 旧再 Activate 新
- [x] 4.5 实现事件路由：将输入事件分发至当前激活工具，回收其刷新指令驱动重绘
- [x] 4.6 约束工具不直接读写文件/全局状态，持久化统一走 `backend.Service`（文档与 lint 提示）
- [x] 4.7 为注册中心编写单元测试（重复 ID、启用过滤、空列表兜底、生命周期顺序）

## 5. 本地后端服务

- [x] 5.1 定义 `backend.Service` 接口（任务提交/取消、KV 读写、关闭），保证可替换为进程间通信
- [x] 5.2 实现「无后端」降级实现，返回「不可用」语义而非 panic
- [x] 5.3 实现异步任务队列：goroutine 执行、结果经 channel/Msg 回传、支持取消
- [x] 5.4 接入 `go.etcd.io/bbolt`，实现按命名分桶的 KV 持久化（读写、隔离、重启可读）
- [x] 5.5 实现错误隔离：单任务 panic 不崩溃应用，转为错误结果 + 结构化 `slog` 日志
- [x] 5.6 接入 `log/slog`，统一结构化日志输出到日志目录
- [x] 5.7 为后端编写单元测试（任务完成/取消、KV 持久化、分桶隔离、panic 隔离）

## 6. 示例工具与集成

- [x] 6.1 实现 `welcome` 示例工具（欢迎页：项目简介、快捷键提示、当前启用工具列表）
- [x] 6.2 实现 `sysinfo` 示例工具（展示 Go 版本、平台、运行时长、目录路径等信息）
- [x] 6.3 在 `internal/tools/builtin` 集中注册示例工具，并设为默认启用
- [x] 6.4 端到端打通：启动应用 → 侧边导航展示工具 → 切换工具 → 生命周期正确触发
- [x] 6.5 验证配置启用列表过滤生效（仅启用部分工具、未知 ID 告警、空列表兜底）

## 7. 工程化与文档

- [x] 7.1 编写 `Makefile`（build/run/test/lint/fmt/install 目标），Windows 提供 `go run` 兜底脚本
- [x] 7.2 配置 `golangci-lint`（gofmt/go vet/ineffassign 等）并接入 `make lint`
- [x] 7.3 配置 `.github/workflows` CI（macOS/Linux/Windows 三平台 build + test + lint）
- [x] 7.4 编写简体中文 `README.md`：项目简介、特性、安装、使用、配置说明、目录结构、开发指南
- [x] 7.5 添加 `.editorconfig`、`.gitignore`（忽略构建产物与本地数据目录）
- [x] 7.6 添加 License 文件并校验全部提交符合 Conventional Commits 风格
- [x] 7.7 全量回归：三平台构建通过、单元测试通过、`worktide` 可正常启动与退出
