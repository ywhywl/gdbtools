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

每行表示一个待创建集群，CSV 字段如下：

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
| `server_type` | 否（见下方说明） | 服务器类型，用于指定预期模版 |

**`server_type` 填写规则**：

- 默认开启 `--auto-select-template`，此时 `server_type` 变为**可选**
- 如果 CSV 填写了 `server_type`，程序会通过 SSH 检测各主机内存，并与 CSV 值比对：
  - 一致 → 正常继续
  - 不一致 → **默认报错退出**，可通过 `--ignore-template-mismatch` 或 `--skip-template-check` 改变行为
- 如果 CSV 未填写 `server_type`，完全以自动检测结果为准

约束：

- `M`、`S` 不能为空
- 同一行内各角色 IP 不能重复
- 同一批次内 `cluster_name` 不能重复
- `server_type` 仅支持：`vm_l`、`vm_m`、`vm_h`、`pm`、`vm_lowercase_0`
- 当前仅支持 `--gtm-use-mode=1`

示例（填写 server_type）：

```csv
num,cluster_name,cluster_group_name,M,S,TS,LS,OS,server_type
1,prod_cluster_full,group_a,172.27.17.25,172.27.21.156,172.27.21.158,,172.27.21.157,vm_l
```

示例（不填写 server_type，完全自动检测）：

```csv
num,cluster_name,cluster_group_name,M,S,TS,LS,OS
1,prod_cluster_full,group_a,172.27.17.25,172.27.21.156,,,172.27.21.157,
```

## 模版自动选择（默认开启）

程序默认通过 SSH 登录各主机，采集内存和虚拟化信息后自动选择模版。

### 内存 → server_type 映射规则

| 虚拟化类型 | 内存 MemGB | 选定 server_type |
|-----------|-----------|-----------------|
| 物理机 (`Virt == "none"`) | 任意 | `pm` |
| 虚拟机 | < 23 | **报错退出** |
| 虚拟机 | >= 23 且 < 30 | `vm_l` |
| 虚拟机 | >= 30 且 < 46 | `vm_m` |
| 虚拟机 | >= 46 | `vm_h` |

### 同集群多 IP 处理

- 同集群所有 IP 必须检测到**相同** server_type，否则报错退出
- 任一 IP SSH 连接失败，整批退出

### 虚拟机内存 < 24G 豁免

通过 `--allow-low-memory-vm` 开关，可将 < 24G 虚拟机的"报错退出"降级为**告警日志 + 使用 vm_l 继续**。

### CSV 指定值与自动检测冲突处理

| `--ignore-template-mismatch` | `--skip-template-check` | 行为 |
|------------------------------|-------------------------|------|
| 未指定 | 未指定 | 不一致则**报错退出**（默认） |
| 指定 | 未指定 | 使用自动检测结果 |
| 未指定 | 指定 | 使用 CSV 指定值 |
| 指定 | 指定 | 两者互斥，报错退出 |

## 模版生成规则

根据最终确定的 `server_type` 自动生成模板名：

| 类型 | 未开启 `--case-sensitive` | 开启 `--case-sensitive` |
|------|--------------------------|------------------------|
| GLOBAL | `template_{server_type}_cluster.json` | `template_{server_type}_lowercase_0_cluster.json` |
| DN | `template_{server_type}_dn.json` | `template_{server_type}_lowercase_0_dn.json` |
| CN | `template_{server_type}_cn.json` | `template_{server_type}_lowercase_0_cn.json` |
| cluster | `template_{server_type}_cluster.json` | `template_{server_type}_lowercase_0_cluster.json` |
| gtm | `template_{server_type}_gtm.json` | `template_{server_type}_lowercase_0_gtm.json` |
| lds | `template_{server_type}_lds.json` | `template_{server_type}_lowercase_0_lds.json` |
| system | `template_{server_type}_system.json` | `template_{server_type}_lowercase_0_system.json` |
| dn_OS | `template_{server_type}_dn_OS.json` | `template_{server_type}_lowercase_0_dn_OS.json` |

例如 `server_type=vm_l` 时：

- 未开启大小写敏感：`template_vm_l_cluster.json`、`template_vm_l_dn.json`、`template_vm_l_cn.json` 等
- 开启大小写敏感：`template_vm_l_lowercase_0_cluster.json`、`template_vm_l_lowercase_0_dn.json`、`template_vm_l_lowercase_0_cn.json` 等

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
| `--insight-user` | Insight 登录用户名，必填 |
| `--insight-password` | Insight 登录密码明文 |
| `--insight-password-b64` | Insight 登录密码 base64 |
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

### 模版自动选择相关参数

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `--auto-select-template` | 通过 SSH 检测内存自动选择模版 | `true`（默认开启） |
| `--ssh-user` | SSH 用户名（开启自动选择时必填） | - |
| `--ssh-key` | SSH 私钥路径，不指定则自动查找 `~/.ssh/id_*` | - |
| `--ssh-password` | SSH 密码明文（key 认证失败时的兜底） | - |
| `--ssh-password-b64` | SSH 密码 base64 | - |
| `--ssh-port` | SSH 端口 | `22` |
| `--ssh-timeout` | 单台 SSH 超时秒数 | `15` |
| `--case-sensitive` | 数据库表名大小写敏感，开启后所有模版名附加 `_lowercase_0` | `false` |
| `--ignore-template-mismatch` | CSV 指定与自动检测不一致时，采用自动检测结果 | `false` |
| `--skip-template-check` | CSV 指定与自动检测不一致时，采用 CSV 指定值 | `false` |
| `--allow-low-memory-vm` | 允许虚拟机内存低于 23G 时不报错，降级使用 vm_l | `false` |

鉴权说明：

- 所有请求头都会带上：
  - `username = --insight-user`
  - `password = base64(--insight-password)` 或 `--insight-password-b64`
- `--ins-user-pwd` / `--ins-user-pwd-base64` 不是 Insight 密码，而是创建集群时写入请求体的业务用户密码

## 使用示例

基本使用（自动检测模版）：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ssh-user deploy \
  --ssh-password 'ssh-password' \
  --ins-user-pwd 'plain-password' \
  --wait-completion \
  --output ./batch_create_result.json
```

使用 `--case-sensitive`（表名大小写敏感）：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ssh-user deploy \
  --ssh-password-b64 'c3NoLXBhc3N3b3Jk' \
  --case-sensitive \
  --ins-user-pwd 'plain-password'
```

允许虚拟机内存低于 24G：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ssh-user deploy \
  --ssh-password 'ssh-password' \
  --allow-low-memory-vm \
  --ins-user-pwd 'plain-password'
```

CSV 指定与自动检测不一致时，采用自动检测结果：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ssh-user deploy \
  --ssh-password 'ssh-password' \
  --ignore-template-mismatch \
  --ins-user-pwd 'plain-password'
```

关闭自动选择（使用 CSV 指定值，恢复旧版行为）：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ins-user-pwd 'plain-password' \
  --auto-select-template=false
```

仅渲染请求体：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ssh-user deploy \
  --ssh-password 'ssh-password' \
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
- `template_selection`（含所有模版名称）
- `status`
- `task_id`
- `attempt`
- `request_payload`
- `response`
- `error`

执行前会打印模版选择汇总表（stderr）：

```
集群           server_type  来源    DN 模版                            CN 模版                            GLOBAL 模版
cluster_01     vm_l         自动    template_vm_l_dn.json              template_vm_l_cn.json              template_vm_l_cluster.json
cluster_02     pm           CSV     template_pm_lowercase_0_dn.json    template_pm_lowercase_0_cn.json    template_pm_lowercase_0_cluster.json
```

## 返回码

- `0`：全部成功，或 `dry-run`
- `1`：部分成功、部分失败
- `2`：全部失败
- `3`：参数错误或执行前校验失败
