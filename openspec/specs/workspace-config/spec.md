## Requirements

### Requirement: 跨平台目录约定

系统 SHALL 在跨平台基础上解析并创建以下目录（缺失时自动创建）：配置目录、数据目录、缓存目录、日志目录。目录路径 MUST 通过 `internal/config` 统一封装，业务代码 SHALL NOT 自行拼接平台相关路径。

#### Scenario: macOS 目录解析

- **WHEN** 应用在 macOS 上首次启动
- **THEN** 配置目录解析为 `~/Library/Application Support/worktide`
- **AND** 数据/缓存目录位于对应平台标准位置

#### Scenario: Linux 目录解析遵循 XDG

- **WHEN** 应用在 Linux 上启动且设置了 `XDG_CONFIG_HOME`
- **THEN** 配置目录解析为 `$XDG_CONFIG_HOME/worktide`

#### Scenario: Windows 目录解析

- **WHEN** 应用在 Windows 上启动
- **THEN** 配置目录解析为 `%AppData%/worktide`

#### Scenario: 目录缺失自动创建

- **WHEN** 解析出的某个目录不存在
- **THEN** 系统以权限 0700 自动创建该目录及必要的父级

### Requirement: 配置文件加载与默认值

系统 SHALL 支持从默认配置文件（`config.yaml`）加载配置，MUST 提供合理的内置默认值，未显式配置的字段 MUST 回退到默认值。配置缺失时应用 MUST 仍能以默认值正常启动。

#### Scenario: 配置文件不存在时使用默认值

- **WHEN** 首次启动且配置目录下无 `config.yaml`
- **THEN** 应用使用内置默认值启动
- **AND** 不向用户报错

#### Scenario: 配置文件存在时合并

- **WHEN** 配置文件存在且包含部分字段
- **THEN** 系统将文件值与默认值合并，文件值优先

### Requirement: 配置项覆盖优先级

系统 SHALL 支持多来源配置，优先级从高到低为：命令行标志 > 环境变量 > 配置文件 > 内置默认值。环境变量 MUST 使用统一前缀（如 `WORKTIDE_`）。

#### Scenario: 环境变量覆盖文件配置

- **WHEN** 环境变量 `WORKTIDE_THEME` 设置为 `dark` 且配置文件中为 `light`
- **THEN** 运行时主题取 `dark`

#### Scenario: 命令行标志覆盖环境变量

- **WHEN** 命令行传入 `--theme=light` 且环境变量为 `dark`
- **THEN** 运行时主题取 `light`

### Requirement: 工具启用列表配置

系统 SHALL 在配置中定义 `tools.enabled` 列表，用于控制加载哪些工具。列表缺失或为空时 MUST 启用默认示例工具。配置中的未知工具 ID MUST 被忽略并记录警告日志。

#### Scenario: 启用列表包含未知工具

- **WHEN** 配置启用列表包含未注册的 ID `foo`
- **THEN** 系统忽略 `foo` 并记录警告日志
- **AND** 其余已注册的启用工具正常加载

### Requirement: 配置变更生效

系统 SHALL 支持配置热加载或重启生效两种模式。主题、快捷键等可热加载项 MUST 在配置变更后即时生效；需要重建后端的项 MUST 提示用户重启。

#### Scenario: 主题热加载

- **WHEN** 用户在外部编辑配置文件修改主题
- **THEN** 应用检测到变更后立即应用新主题，无需重启
