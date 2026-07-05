## Requirements

### Requirement: 工具自描述注册

系统 SHALL 定义统一的 `Tool` 接口，工具 MUST 通过元信息（ID、显示名、描述、图标、版本）自描述，并通过显式 `Register` 调用登记到全局注册中心。工具 ID MUST 全局唯一且使用 kebab-case。

#### Scenario: 注册一个新工具

- **WHEN** 开发者在内置工具集合中调用 `Register` 登记一个实现 `Tool` 接口的工具
- **THEN** 该工具出现在注册中心的工具列表中
- **AND** 其元信息可被主壳读取用于导航展示

#### Scenario: 重复 ID 注册被拒绝

- **WHEN** 开发者尝试以已存在的 ID 注册另一个工具
- **THEN** 注册中心返回错误并拒绝注册
- **AND** 应用启动日志记录该冲突

### Requirement: 按配置启用与禁用工具

系统 SHALL 根据用户配置的「启用列表」决定加载哪些已注册工具，未在启用列表中的工具 MUST NOT 出现在导航与运行时中。启用列表为空时 MUST 至少加载一个默认示例工具以保证界面可用。

#### Scenario: 仅启用部分工具

- **WHEN** 配置启用列表只包含 `welcome` 与 `sysinfo`
- **THEN** 侧边导航仅出现这两个工具
- **AND** 其他已注册工具不被实例化

#### Scenario: 启用列表为空兜底

- **WHEN** 配置启用列表为空或缺失
- **THEN** 系统加载默认示例工具，主界面保持可用

### Requirement: 工具生命周期

每个被启用的工具 MUST 按统一生命周期管理：初始化（Init）、激活（Activate）、停用（Deactivate）、关闭（Close）。系统 MUST 保证同一时刻仅一个工具处于「激活」状态，切换工具时先停用旧工具再激活新工具。

#### Scenario: 切换工具触发生命周期

- **WHEN** 用户从工具 A 切换到工具 B
- **THEN** 系统 先调用 A 的 Deactivate，再调用 B 的 Activate

#### Scenario: 退出时释放资源

- **WHEN** 应用正常退出
- **THEN** 系统对当前激活工具调用 Deactivate 与 Close，释放其持有的资源

### Requirement: 工具事件路由

系统 SHALL 将用户的键鼠输入事件路由到当前激活工具的事件处理方法，工具返回的命令与结果 MUST 通过统一消息机制回传主壳驱动刷新。工具 MUST NOT 直接操作其他工具或全局状态。

#### Scenario: 输入事件送达激活工具

- **WHEN** 用户在工具 B 视图中按键
- **THEN** 该事件被路由到工具 B 的事件处理方法
- **AND** 返回的刷新指令触发主内容区重绘

### Requirement: 工具依赖后端经统一接口

工具需要异步任务或数据持久化时 MUST 通过 `backend.Service` 接口访问，SHALL NOT 绕过后端直接读写文件或全局变量，以保证可替换与可测试。

#### Scenario: 工具请求异步任务

- **WHEN** 工具需要执行耗时操作
- **THEN** 工具通过 `backend.Service` 提交任务
- **AND** 任务完成后结果经消息机制回传，UI 不被阻塞
