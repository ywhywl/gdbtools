# GoldenDB 工具接口集抽象

## 目标

把 [ZXCLOUDGoldenDB分布式数据库通用rest接口操作指南.txt](/Users/wenlongy/dev/src/gdbtools/ZXCLOUDGoldenDB分布式数据库通用rest接口操作指南.txt) 抽象成一套可复用的工具接口集，方便后续由程序、Agent 或平台统一调取。

机器可读注册表见：
- [goldendb_tool_interface_set.yaml](/Users/wenlongy/dev/src/gdbtools/docs/goldendb_tool_interface_set.yaml)

## 统一约定

- 所有接口统一使用请求头鉴权：
  - `username`
  - `password`
- `password` 需要 `base64` 编码。
- 通用返回结构统一包含：
  - `code`
  - `msg`
  - `data`
  - `duration`
  - `operationId`

## 推荐抽象方式

建议按 3 层使用：

1. 元数据发现层
   - `rest_tree_list`
   - `rest_info_get`
   - `rest_version_update`

2. 业务执行层
   - 主机管理
   - 租户生命周期
   - 备份恢复
   - 用户权限
   - 监控采集
   - 高可用切换
   - 巡检与系统任务

3. 异步任务跟踪层
   - 对返回 `taskId` 的动作，统一通过 `async_pairs` 中的 query 接口查询结果

## 最关键的设计点

- 不手工把所有字段固定死到代码里。
- 把“接口目录”和“接口详情”作为元工具保留。
- 业务工具集只负责：
  - 提供稳定名字
  - 标注 method/path
  - 标注所属域
  - 标注是否异步
  - 标注对应进度查询接口

这样后续如果 GoldenDB 接口字段有变动，仍然可以先通过 `rest_info_get` 获取最新 `requestExample`、`requestDesc`、`responseDesc`，再发起真实调用。

## 工具集分组

当前已抽象以下分组：

- `interface_catalog`
- `host_management`
- `parameter_template`
- `tenancy_lifecycle`
- `backup_restore`
- `tenancy_runtime_config`
- `db_user_permission`
- `tenancy_business`
- `lds_management`
- `gtm_cluster`
- `ha_switch`
- `monitor_collect`
- `manager_node`
- `self_healing`
- `sql_blacklist`
- `ha_strategy_template`
- `device_standard`
- `advanced_statistics`
- `system_patrol`
- `version_repository`
- `common_command`
- `network_info`

## 后续接入建议

如果后面要直接落代码，建议实现一个统一客户端：

- `LoginHeadersProvider`
- `CatalogClient`
- `ToolRegistry`
- `GenericInvoker`
- `TaskPollingClient`

最小执行流程：

1. 从注册表按 `tool name` 找到 `method/path`
2. 通过 `rest_info_get` 获取当前版本接口元数据
3. 按元数据校验请求参数
4. 发起调用
5. 如果返回 `taskId`，转到对应 query 工具轮询

## 当前产物边界

这次抽象完成的是“工具接口集注册表”，不是完整 SDK。

也就是说现在已经有：

- 接口分组
- 工具名
- 方法
- 路径
- 异步映射
- 通用鉴权和返回约定

如果你下一步需要，我可以继续直接补：

- Go 版 `GoldenDB REST client` 代码骨架
- `tool name -> HTTP request` 的执行器
- `taskId` 轮询器
- OpenAPI 风格 JSON/YAML 导出
