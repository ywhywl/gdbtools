# insight-batch-add-dn 使用说明

## 功能说明

`insight-batch-add-dn` 用于批量向已有 GoldenDB 集群新增 DN 组件。

执行流程：

1. 解析输入文件
2. 按 `insight_addr + cluster_name` 分组
3. 查询每组的 `clusterId`
4. 若未提供 `dbgroup_id`，则根据 `dbgroup_name` 查询
5. 调用 `/open_api/insight/external/install/batchAddSlaveDN`
6. 轮询 `/open_api/insight/external/install/querybatchAddSlaveDNResult`
7. 汇总每行结果

命令入口：

```bash
go run ./cmd/insight-batch-add-dn --help
```

## 输入格式

支持 `CSV` 和 `JSON`。

必填字段：

| 字段名 | 是否必填 | 说明 |
| --- | --- | --- |
| `insight_addr` | 是 | Insight 地址 |
| `cluster_name` | 是 | 集群名称 |
| `template_name` | 与 `server_type` 二选一 | 普通 DN 模板文件名；OS 节点会自动使用对应的 `_dn_OS.json` 模板 |
| `server_type` | 与 `template_name` 二选一 | `pm`、`vm_l`、`vm_m`、`vm_h`、`vm_lowercase_0`；用于自动生成模板名 |
| `role` | 否 | `OS` 节点使用 OS 专用模板；其他角色使用普通 DN 模板 |
| `ip` | 是 | DN 所在主机 IP |
| `team_id` | 是 | 目标 team ID，必须人工指定 |

以下两者至少提供一个：

- `dbgroup_name`
- `dbgroup_id`

可选字段：

- `port`
- `backup_select_strategy`
- `backup_start_time`
- `backup_end_time`
- `backup_id`
- `admin_port`
- `install_user`
- `install_path`
- `data_path`
- `log_path`

CSV 示例：

```csv
insight_addr,cluster_name,server_type,role,dbgroup_name,team_id,ip,port,admin_port,install_user,install_path,data_path,log_path
10.0.0.10:8444,prod_cluster_a,vm_l,M,group_1,1,10.0.0.41,,5501,nudb1,/data/goldendb/nudb1,/data/goldendb/nudb1/data,/data/goldendb/nudb1/log
```

## 参数说明

| 参数 | 说明 |
| --- | --- |
| `--input` | 输入文件路径，必填 |
| `--insight-user` | Insight 登录用户名，必填 |
| `--insight-password` | Insight 登录密码明文 |
| `--insight-password-b64` | Insight 登录密码 base64 |
| `--default-port` | 默认 DN 端口 |
| `--default-admin-port` | 默认 DN 管理端口 |
| `--default-install-user` | 默认安装用户 |
| `--default-install-path` | 默认安装路径 |
| `--default-data-path` | 默认数据路径 |
| `--default-log-path` | 默认日志路径 |
| `--prefix` | 自动生成安装用户名的前缀，默认 `nu` |
| `--base-path` | 自动生成安装路径的根目录，默认 `/data/goldendb` |
| `--case-sensitive` | 自动生成大小写敏感模板名 |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--poll-timeout` | 轮询超时秒数，默认 `3600` |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |
| `--output-json` | 输出 JSON |
| `--debug` | 打印实际请求 URL、请求体和响应的 debug 日志，默认关闭 |

说明：

- 所有请求头统一带 `username/password`
- `password` 取 Insight 登录密码的 base64
- 若未提供 `dbgroup_id`，命令会使用 `dbgroup_name` 查询并回填
- `team_id` 必须人工指定，程序不会自动分配
- 相同 `dbgroup_id + team_id + backupTask` 的行会被合并到同一个 `teamList` 项中
- 模板写入每个 `dnList` 节点：普通节点为 `template_{server_type}_dn.json`，OS 节点为 `template_{server_type}_dn_OS.json`
- `install_path` 未提供时默认为 `{base-path}/{install_user}`；`data_path` 默认为 `{install_path}/data`；`log_path` 默认为 `{install_path}/log`

## backupTask 组装规则

当 `backup_select_strategy` 非空时，会生成：

```json
{
  "selectStrategy": 1,
  "startTime": "00:00",
  "endTime": "06:00",
  "backupId": "backup-task-1"
}
```

其中 `startTime`、`endTime`、`backupId` 仅在提供时写入。

## 使用示例

```bash
go run ./cmd/insight-batch-add-dn \
  --input ./batch_add_dn.csv \
  --insight-user admin \
  --insight-password 'insight-password' \
  --default-admin-port 5501 \
  --debug \
  --output-json
```

安装进度默认输出到 stderr；`--debug` 额外输出实际请求 URL、请求体和响应体，可用于和内网接口文档或抓包结果逐字段对比。DN 请求体不再包含顶层 `parameterTemplateInfos`，模板位于每个 `dnList` 节点：

```json
"dnList": [
  {"ip": "10.0.0.41", "templateName": "template_vm_l_dn.json"},
  {"ip": "10.0.0.42", "templateName": "template_vm_l_dn_OS.json"}
]
```

## 输出结果

输出结构包含：

- `summary`
- `groups`
- `results`

每个 `groups` 项包含：

- `insight_addr`
- `cluster_name`
- `task_id`
- `total`
- `success_count`
- `failed_count`
- `status`
- `items`

每个 `results` 项包含：

- `row_no`
- `dbgroup_id`
- `team_id`
- `ip`
- `port`
- `status`
- `message`

## 返回码

- `0`：命令执行完成
- `2`：参数错误、输入校验失败或接口调用失败
