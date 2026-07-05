## ADDED Requirements

### Requirement: 后端服务抽象与按需启用

系统 SHALL 在 `internal/backend` 提供统一的 `Service` 接口，封装异步任务、缓存与持久化能力。后端 MUST 是可选的，配置中未启用时应用 MUST 仍能以「无后端」模式启动并提供降级体验。

#### Scenario: 后端未启用降级运行

- **WHEN** 配置中后端被设为禁用
- **THEN** 应用以无后端模式启动
- **AND** 调用后端能力的工具得到「不可用」语义的返回而非崩溃

#### Scenario: 后端启用并就绪

- **WHEN** 配置启用后端且启动完成
- **THEN** 后端服务初始化完成并对外提供任务/缓存/持久化接口

### Requirement: 异步任务队列

后端 SHALL 提供异步任务提交接口，任务 MUST 在独立 goroutine 执行，执行结果通过消息机制回传。任务 MUST 不阻塞 UI 主循环，且 MUST 支持取消。

#### Scenario: 提交并完成异步任务

- **WHEN** 工具通过后端提交一个耗时任务
- **THEN** UI 不被阻塞，任务在后台执行
- **AND** 任务完成后结果消息回传给提交方

#### Scenario: 取消正在执行的任务

- **WHEN** 工具请求取消一个已提交未完成的任务
- **THEN** 后端停止该任务并回传取消结果

### Requirement: 键值持久化

后端 SHALL 提供嵌入式键值（KV）持久化能力（基于 bbolt），工具 MUST 能按命名分桶读写数据。持久化数据 MUST 在应用重启后仍然存在。

#### Scenario: 写入并重新读取数据

- **WHEN** 工具向桶 `notes` 写入键 `k1` 值 `v1` 后重启应用再读取 `k1`
- **THEN** 读取结果为 `v1`

#### Scenario: 命名分桶隔离

- **WHEN** 工具 A 向桶 `a` 写入键 `k`，工具 B 向桶 `b` 写入相同键 `k`
- **THEN** 两个桶互不干扰，各自读取到各自的值

### Requirement: 后端与 TUI 解耦

后端 MUST 以 Go interface 形式向工具暴露能力，TUI 与工具 SHALL 通过接口与消息（channel/Msg）与后端通信，SHALL NOT 直接共享可变状态。该解耦 MUST 使后端可在未来替换为独立进程而不修改上层。

#### Scenario: 通过接口访问后端

- **WHEN** 工具需要后端能力
- **THEN** 工具仅依赖 `backend.Service` 接口类型，不依赖具体实现

#### Scenario: 后端实现可替换

- **WHEN** 将后端实现从同进程替换为进程间通信版本
- **THEN** 工具代码无需修改即可正常工作

### Requirement: 后端错误隔离

后端发生的错误 MUST 不导致整个应用崩溃，MUST 被捕获并转化为可处理的错误结果回传调用方，同时记录结构化日志。

#### Scenario: 后端任务 panic 被隔离

- **WHEN** 某后端任务执行过程中发生 panic
- **THEN** 应用继续运行不崩溃
- **AND** 调用方收到错误结果，结构化日志记录该异常
