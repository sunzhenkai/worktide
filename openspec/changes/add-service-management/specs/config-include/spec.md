## ADDED Requirements

### Requirement: include 字段识别

`config.yaml` SHALL 支持顶层 `include:` 字段，类型为字符串数组。每个元素 MUST 是一个 YAML 文件路径，指向需要被合并进主配置的文件。

#### Scenario: 单个 include

- **WHEN** `config.yaml` 含 `include: [~/work/api/.worktide.yaml]`
- **THEN** 加载器按顺序读取该文件并与主配置 deep merge

#### Scenario: 多个 include

- **WHEN** `config.yaml` 含 `include: [a.yaml, b.yaml, c.yaml]`
- **THEN** 加载器依次读取 a.yaml → b.yaml → c.yaml 并合并到主配置

#### Scenario: 无 include 字段

- **WHEN** `config.yaml` 不含 `include:` 字段
- **THEN** 加载行为不变，仅使用 `config.yaml` 自身与 `services.yaml`

### Requirement: 路径 ~ 展开

include 路径 MUST 支持以 `~` 开头的用户主目录引用，并在解析前展开为绝对路径。

#### Scenario: ~ 单用

- **WHEN** `include: [~/.config/worktide/includes/team.yaml]`
- **THEN** 该路径被展开为 `<os.UserHomeDir()>/.config/worktide/includes/team.yaml`

#### Scenario: ~ 中段

- **WHEN** `include: [/some/abs/~/inner.yaml]`（中段含 ~）
- **THEN** 仅前导 `~/...` 被展开；中段字面保留

### Requirement: 路径 ${ENV} 展开

include 路径 SHALL 支持 `${VAR}` 与 `${VAR:-default}` 形式的 shell 风格环境变量引用。

#### Scenario: 简单变量

- **WHEN** `include: ["${WORKTIDE_INCLUDES}/api.yaml"]`，环境变量 `WORKTIDE_INCLUDES=/Users/me/includes`
- **THEN** 路径被展开为 `/Users/me/includes/api.yaml`

#### Scenario: 变量未设置报错

- **WHEN** `include: ["${WORKTIDE_INCLUDES}/api.yaml"]`，环境变量 `WORKTIDE_INCLUDES` 未设置
- **THEN** 加载以非零退出码失败
- **AND** stderr 包含「environment variable WORKTIDE_INCLUDES is not set」

#### Scenario: 带默认值变量

- **WHEN** `include: ["${WORKTIDE_INCLUDES:-/etc/worktide}/api.yaml"]`，变量未设置
- **THEN** 路径展开为 `/etc/worktide/api.yaml`

### Requirement: 路径缺失与错误处理

include 路径指向的文件 MUST 存在且为普通文件；缺失或类型不符 MUST 视为加载错误并中止启动（不允许静默忽略）。

#### Scenario: 文件不存在

- **WHEN** include 指向不存在的路径
- **THEN** 加载以非零退出码失败
- **AND** stderr 包含路径与「file not found」

#### Scenario: 目录而非文件

- **WHEN** include 指向目录
- **THEN** 加载以非零退出码失败
- **AND** stderr 包含「is a directory」

#### Scenario: 文件不可读

- **WHEN** 文件存在但当前用户无读权限
- **THEN** 加载以非零退出码失败
- **AND** stderr 透传底层错误

### Requirement: 循环 include 检测

加载器 MUST 检测通过 include 链形成的循环引用；超过 8 层 include 嵌套 MUST 视为错误。

#### Scenario: 直接循环

- **WHEN** a.yaml 含 `include: [b.yaml]`，b.yaml 含 `include: [a.yaml]`
- **THEN** 加载以非零退出码失败
- **AND** stderr 包含「include cycle detected」与路径

#### Scenario: 嵌套过深

- **WHEN** include 链形成超过 8 层的嵌套
- **THEN** 加载以非零退出码失败
- **AND** stderr 包含「include depth exceeded」

#### Scenario: 同文件多次出现不构成循环

- **WHEN** 同一文件被多次引用但链路无环（如 `include: [a.yaml, a.yaml]`）
- **THEN** 仅合并一次（visited-set 去重）

### Requirement: deep merge 合并顺序与规则

include 文件 MUST 与主配置执行 deep merge；合并顺序 MUST 为：内置默认值 → `config.yaml` 顶层 → include 列表按出现顺序 → `services.yaml`（最终覆盖）。

#### Scenario: map 逐 key 合并

- **WHEN** 主配置 `services.api = { cwd: /A, command: make }`，include 文件 `services.web = { cwd: /W, command: pnpm }`
- **THEN** 合并后 `services` 含 `api` 与 `web` 两条

#### Scenario: 同 key 后者覆盖

- **WHEN** 主配置 `services.api.cwd = /A`，include 文件 `services.api.cwd = /B`
- **THEN** 合并后 `services.api.cwd = /B`

#### Scenario: list 后者完全替换

- **WHEN** 主配置 `tools.enabled: [welcome]`，include 文件 `tools.enabled: [welcome, sysinfo]`
- **THEN** 合并后 `tools.enabled = [welcome, sysinfo]`，不追加

#### Scenario: services.yaml 最终覆盖

- **WHEN** config.yaml `services.api.cwd = /A`，services.yaml `services.api.cwd = /B`
- **THEN** 合并后 `services.api.cwd = /B`

### Requirement: 字段类型校验

合并过程中 MUST 校验：被合并的字段在主配置与 include 中必须保持相同的「map/list/scalar」类型，否则视为加载错误。

#### Scenario: 类型冲突报错

- **WHEN** 主配置 `theme.name = "dark"`（scalar），include 文件 `theme.name = { nested: true }`（map）
- **THEN** 加载以非零退出码失败
- **AND** stderr 包含字段路径与「type mismatch」

#### Scenario: 同类型合并

- **WHEN** 主配置 `theme.name = "dark"`，include 文件 `theme.name = "light"`
- **THEN** 合并后 `theme.name = "light"`

### Requirement: 热加载触发

include 文件的变更 SHALL 通过 fsnotify 触发现有 Config watcher；触发后 MUST 重新走完整合并流程（再次解析 include 链）。

#### Scenario: 修改 include 文件触发重载

- **WHEN** TUI 运行中，用户修改 include 引用的文件并保存
- **THEN** watcher 收到 fsnotify 事件
- **AND** 内存中的 Config 被重建并通过 `OnReload` 回调推送给已注册的监听者

#### Scenario: 修改期间 include 失败

- **WHEN** include 引用的文件被改成无效 YAML
- **THEN** watcher 输出 WARN 级别日志，本次变更不应用
- **AND** 内存中保留上一次的可用配置

#### Scenario: 已在跑的进程不受影响

- **WHEN** include 文件变更触发 services map 更新
- **THEN** 已经在 running 状态的进程不被重启或终止
- **AND** TUI 顶部可选地展示「配置已变更，是否重启受影响服务」

### Requirement: services.yaml 不参与 include

`services.yaml` MUST NOT 通过 `include:` 字段引用其他文件；如检测到该字段 MUST 忽略并 WARN，避免 CLI 写盘区域被外部 include 污染。

#### Scenario: services.yaml 含 include 字段

- **WHEN** `services.yaml` 含 `include: [extra.yaml]`
- **THEN** 加载时忽略 `include` 字段
- **AND** 输出 WARN 级别日志

#### Scenario: services.yaml 不含 include

- **WHEN** `services.yaml` 不含 `include` 字段
- **THEN** 加载行为不变
