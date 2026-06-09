# insight-batch-create 使用说明

## 功能说明

`insight-batch-create` 用于根据 CSV 清单批量调用 GoldenDB 新增租户接口。

- 创建接口：`/open_api/insight/external/tenant/createCluster`
- 进度查询接口：`/open_api/insight/external/tenant/getInstallClusterProcess?taskId=...`

命令入口：

```bash
go run ./cmd/insight-batch-create --help
```

## CSV 字段说明

每行表示一个待创建集群，CSV 必须包含以下字段：

| 字段名 | 是否必填 | 说明 |
| --- | --- | --- |
| `num` | 是 | 序号，仅用于结果展示 |
| `cluster_name` | 是 | 集群名称 |
| `cluster_group_name` | 是 | 集群分组名称，仅用于结果展示 |
| `M` | 是 | 主节点 IP |
| `S` | 是 | 备节点 IP |
| `TS` | 否 | TS 角色节点 IP |
| `LS` | 否 | LS 角色节点 IP |
| `OS` | 否 | OS 角色节点 IP |
| `server_type` | 是 | 服务器类型，用于自动扩展模板名称 |

约束：

- `M`、`S` 不能为空
- 同一行内各角色 IP 不能重复
- 同一批次内 `cluster_name` 不能重复
- `server_type` 仅支持：`vm_l`、`vm_m`、`vm_h`、`pm`、`vm_lowercase_0`
- 当前仅支持 `--gtm-use-mode=1`

示例：

```csv
num,cluster_name,cluster_group_name,M,S,TS,LS,OS,server_type
1,prod_cluster_full,group_a,172.27.17.25,172.27.21.156,172.27.21.158,,172.27.21.157,vm_l
```

## 模板生成规则

根据 `server_type` 自动生成模板名：

- `global_template = template_{server_type}_cluster`
- `dn_template = template_{server_type}_dn`
- `cn_template = template_{server_type}_cn`
- `os_cn_template = template_{server_type}_cn_OS`

例如 `server_type=vm_l` 时：

- `template_vm_l_cluster`
- `template_vm_l_dn`
- `template_vm_l_cn`
- `template_vm_l_cn_OS`

## 节点生成规则

CN 规则：

- 每个非空角色 IP 生成两个 CN：
- `dbproxy1`，`servicePort=3306`
- `dbproxy2`，`servicePort=3307`
- 安装用户为 `{prefix}dbproxy1`、`{prefix}dbproxy2`

DN 规则：

- 每个非空角色 IP 生成一个 DN
- 安装用户固定为 `{prefix}db1`
- 安装路径为 `{base-path}/{prefix}db1`
- 数据路径为 `{base-path}/{prefix}db1/data`

DN `dbRole` 映射：

- `M -> 1`
- `S -> 0`
- `TS -> 0`
- `LS -> 0`
- `OS -> 2`

DN `teamId` 映射：

- `M -> 1`
- `S -> 2`
- `LS -> 3`
- `OS -> 4`
- `TS -> 5`

## 常用参数

| 参数 | 说明 |
| --- | --- |
| `--api` | Insight 地址，必填 |
| `--csv` | CSV 文件路径，必填 |
| `--prefix` | 安装用户名前缀，默认 `nu` |
| `--base-path` | 安装根目录，默认 `/data/goldendb` |
| `--ins-user-pwd` | 业务用户密码明文 |
| `--ins-user-pwd-base64` | 业务用户密码 base64 |
| `--ha-mode` | 高可用模式，默认 `0` |
| `--instance-type` | 实例类型，默认 `1` |
| `--charset` | 字符集，默认 `utf8mb4` |
| `--mode` | 安装模式，默认 `1` |
| `--gtm-use-mode` | GTM 使用模式，默认 `1` |
| `--cluster-desc` | 集群描述，默认取 `cluster_name` |
| `--wait-completion` | 提交后等待安装完成 |
| `--max-wait-time` | 最大等待秒数，默认 `3600` |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--max-retries` | 失败重试次数，默认 `1` |
| `--no-verify` | 跳过 SSL 证书校验，默认开启 |
| `--verify-ssl` | 启用 SSL 证书校验 |
| `--dry-run` | 只渲染请求体，不发起接口调用 |
| `--output` | 将结构化结果写入 JSON 文件 |
| `--format` | 输出格式：`json` 或 `text` |

## 使用示例

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --csv ./clusters.csv \
  --ins-user-pwd 'plain-password' \
  --wait-completion \
  --output ./batch_create_result.json
```

仅渲染请求体：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --csv ./clusters.csv \
  --ins-user-pwd 'plain-password' \
  --dry-run
```

## 输出结果

默认输出 JSON，包含：

- `success`
- `api`
- `summary`
- `clusters`

其中每个 `clusters` 项包含：

- `row_no`
- `num`
- `cluster_name`
- `cluster_group_name`
- `server_type`
- `template_selection`
- `status`
- `task_id`
- `attempt`
- `request_payload`
- `response`
- `error`

## 返回码

- `0`：全部成功，或 `dry-run`
- `1`：部分成功、部分失败
- `2`：全部失败
- `3`：参数错误或执行前校验失败
