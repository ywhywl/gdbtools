# insight-batch-add-dn 使用说明

## 功能说明

`insight-batch-add-dn` 用于批量向已有 GoldenDB 集群新增 DN 组件。

执行流程：

1. 解析输入文件
2. 按 `insight_addr + cluster_name + template_name` 分组
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
| `template_name` | 是 | DN 模板名 |
| `ip` | 是 | DN 所在主机 IP |

以下两者至少提供一个：

- `dbgroup_name`
- `dbgroup_id`

可选字段：

- `team_id`
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
insight_addr,cluster_name,template_name,dbgroup_name,team_id,ip,port,admin_port,install_user,install_path,data_path,log_path
10.0.0.10:8444,prod_cluster_a,template_vm_l_dn,group_1,1,10.0.0.41,0,5501,nudb1,/data/goldendb/nudb1,/data/goldendb/nudb1/data,/data/goldendb/nudb1/log
```

## 参数说明

| 参数 | 说明 |
| --- | --- |
| `--input` | 输入文件路径，必填 |
| `--default-port` | 默认 DN 端口 |
| `--default-admin-port` | 默认 DN 管理端口 |
| `--default-install-user` | 默认安装用户 |
| `--default-install-path` | 默认安装路径 |
| `--default-data-path` | 默认数据路径 |
| `--default-log-path` | 默认日志路径 |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--poll-timeout` | 轮询超时秒数，默认 `3600` |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |
| `--output-json` | 输出 JSON |

说明：

- 若未提供 `dbgroup_id`，命令会使用 `dbgroup_name` 查询并回填
- 相同 `dbgroup_id + team_id + backupTask` 的行会被合并到同一个 `teamList` 项中

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
  --default-admin-port 5501 \
  --output-json
```

## 输出结果

输出结构包含：

- `summary`
- `groups`
- `results`

每个 `groups` 项包含：

- `insight_addr`
- `cluster_name`
- `template_name`
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
