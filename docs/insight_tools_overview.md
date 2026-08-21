# Insight 工具总览

## 说明

本文档汇总 `gdbtools` 中与 GoldenDB Insight OpenAPI 相关的 5 个命令，并按常见交付流程组织：

1. 主机纳管
2. 建集群
3. 扩容 CN
4. 扩容 DN
5. 初始化管理员用户

这些命令都采用独立二进制入口，适合单独执行，也适合被平台流程或自动化脚本串联调用。

## 统一鉴权

这 5 个命令统一使用 Insight 登录信息做请求头鉴权：

- `--insight-user`
- `--insight-password`
- `--insight-password-b64`

请求头格式：

- `username = --insight-user`
- `password = base64(--insight-password)` 或 `--insight-password-b64`

注意：

- 这套鉴权参数适用于所有命令
- 某些业务命令还会额外有“数据库用户密码”或“业务用户密码”，这些不是 Insight 密码

## 命令清单

| 场景 | 命令 | 文档 |
| --- | --- | --- |
| 主机纳管 | `insight-batch-onboard-hosts` | [insight_batch_onboard_hosts.md](insight_batch_onboard_hosts.md) |
| 批量建集群 | `insight-batch-create` | [insight_batch_create.md](insight_batch_create.md) |
| 批量新增 CN | `insight-batch-add-cn` | [insight_batch_add_cn.md](insight_batch_add_cn.md) |
| 批量新增 DN | `insight-batch-add-dn` | [insight_batch_add_dn.md](insight_batch_add_dn.md) |
| 初始化管理员用户 | `insight-create-dbmgr` | [insight_create_dbmgr.md](insight_create_dbmgr.md) |

## 一、主机纳管

命令：

```bash
go run ./cmd/insight-batch-onboard-hosts --help
```

用途：

- 批量调用 Insight 主机纳管接口
- 将服务器主机分批纳入平台管理
- 支持 CSV 和 JSON 输入

适用时机：

- 新服务器刚准备好，需要先让 Insight 识别和接管
- 后续建集群或扩容前，需要确认目标主机已纳管

典型输入：

- `room_name`
- `server_ip`
- `install_path`
- `data_path`
- `system_parameter`
- `region`

典型输出：

- 每台主机的纳管状态：`success` / `failed`

## 二、建集群

命令：

```bash
go run ./cmd/insight-batch-create --help
```

用途：

- 根据 CSV 批量创建 GoldenDB 集群
- 自动生成 DN/CN 模板组合
- 自动生成安装节点列表
- 可选等待安装完成

适用时机：

- 新建一批 GoldenDB 集群
- 按标准化模板批量初始化租户

典型输入：

- `cluster_name`
- `cluster_group_name`
- `M/S/TS/LS/OS`
- `server_type`

关键特点：

- 基于 `server_type` 自动推导模板名
- 自动生成 `cnInstallList` 和 `dnInstallList`
- 支持 `dry-run`
- 支持失败重试和安装进度轮询
- `--ins-user-pwd` / `--ins-user-pwd-base64` 是业务用户密码，不是 Insight 鉴权密码

典型输出：

- 每个集群的请求体
- `task_id`
- 安装结果
- 模板选择信息

## 三、扩容 CN

命令：

```bash
go run ./cmd/insight-batch-add-cn --help
```

用途：

- 批量向已有集群新增 CN 组件
- 按集群和模板分组提交安装任务

适用时机：

- 某个集群需要增加新的 CN 节点
- 对多个集群执行同类 CN 扩容

典型输入：

- `insight_addr`
- `cluster_name`
- `template_name`
- `ip`
- `port`
- `install_user`
- `install_path`
- `service_port`

关键特点：

- 先按 `(insight_addr, cluster_name, template_name)` 分组
- 自动将 `cluster_name` 解析成 `clusterId`
- 轮询新增 CN 任务结果

典型输出：

- 每行 CN 安装结果
- 每个分组任务的成功/失败统计

## 四、扩容 DN

命令：

```bash
go run ./cmd/insight-batch-add-dn --help
```

用途：

- 批量向已有集群新增 DN 组件
- 支持按分片、团队和备份策略进行组织

适用时机：

- 某个集群需要补充 DN 节点
- 某个分片组需要新增从 DN
- 按备份策略批量扩容 DN

典型输入：

- `insight_addr`
- `cluster_name`
- `template_name`
- `dbgroup_name` 或 `dbgroup_id`
- `team_id`
- `ip`
- `port`
- `admin_port`
- `install_user`
- `install_path`
- `data_path`
- `log_path`

关键特点：

- 若未提供 `dbgroup_id`，会用 `dbgroup_name` 自动查询
- 相同 `dbgroup_id + team_id + backupTask` 会合并到同一个 `teamList`
- 支持备份策略字段：
  - `backup_select_strategy`
  - `backup_start_time`
  - `backup_end_time`
  - `backup_id`

典型输出：

- 每行 DN 安装结果
- 每个分组任务的成功/失败统计

## 五、初始化管理员用户

命令：

```bash
go run ./cmd/insight-create-dbmgr --help
```

用途：

- 为指定集群创建管理员用户
- 执行授权语句

适用时机：

- 新集群创建完成后，初始化管理账号
- 对某个已有集群补建 `dbmgr` 用户

典型输入：

- `cluster_name`
- `user_name`
- `user_host`
- `db_user_password` 或 `db_user_password_b64`
- `grant_file`

关键特点：

- 自动通过 `cluster_name` 查询 `clusterId`
- 请求头使用 Insight 登录鉴权
- 支持两种 `grant-file` 格式：
  - JSON 数组
  - `{"grantList":[...]}`

典型输出：

- 直接打印接口返回 JSON

## 典型执行顺序

一个比较常见的标准流程如下：

1. 主机纳管  
   先执行 `insight-batch-onboard-hosts`，确保服务器已被 Insight 纳管。

2. 建集群  
   再执行 `insight-batch-create`，批量创建 GoldenDB 集群。

3. 初始化管理员用户  
   集群创建完成后，执行 `insight-create-dbmgr` 初始化管理用户。

4. 后续扩容  
   当需要增加计算或存储节点时：
   - 使用 `insight-batch-add-cn` 扩容 CN
   - 使用 `insight-batch-add-dn` 扩容 DN

## 公共行为说明

这些命令共享一组公共行为：

- 自动规范化 `api/insight_addr`
- 本地地址默认 `http`，其他地址默认 `https`
- 支持跳过 SSL 证书校验
- HTTPS 协议错误时自动回退 HTTP
- 支持 `CSV` / `JSON` 表格输入
- 自动处理 `UTF-8 BOM`

## 入口位置

命令入口位于：

- [cmd/insight-batch-onboard-hosts](../cmd/insight-batch-onboard-hosts)
- [cmd/insight-batch-create](../cmd/insight-batch-create)
- [cmd/insight-batch-add-cn](../cmd/insight-batch-add-cn)
- [cmd/insight-batch-add-dn](../cmd/insight-batch-add-dn)
- [cmd/insight-create-dbmgr](../cmd/insight-create-dbmgr)

公共实现位于：

- [internal/insightopen](../internal/insightopen)
- [internal/insightinput](../internal/insightinput)
