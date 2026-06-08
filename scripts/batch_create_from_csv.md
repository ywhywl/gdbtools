# batch_create_from_csv.py 使用说明

## 功能说明

`batch_create_from_csv.py` 用于根据 CSV 清单批量调用 GoldenDB 新增租户接口：

- 创建接口：`/open_api/insight/external/tenant/createCluster`
- 进度查询接口：`/open_api/insight/external/tenant/getInstallClusterProcess?taskId=...`

脚本支持独立执行，也兼容平台侧通过 `SCRIPT_DB_INSTALL_STEP1` 调用。

## CSV 字段说明

每行表示一个待创建集群，CSV 必须包含以下字段：

| 字段名 | 是否必填 | 说明 |
| --- | --- | --- |
| `num` | 是 | 序号，仅用于结果展示 |
| `cluster_name` | 是 | 集群名称 |
| `cluster_group_name` | 是 | 集群分组名称，仅用于结果展示和平台后续元数据处理 |
| `M` | 是 | 主节点 IP |
| `S` | 是 | 备节点 IP |
| `TS` | 否 | TS 角色节点 IP |
| `LS` | 否 | LS 角色节点 IP |
| `OS` | 否 | OS 角色节点 IP |
| `server_type` | 是 | 服务器类型，用于自动扩展模板名称 |

约束：

- `M`、`S` 不能为空
- 同一行内各角色 IP 不能重复
- `server_type` 当前仅支持：`vm_l`、`vm_m`、`vm_h`、`pm`、`vm_lowercase_0`

示例：

```csv
num,cluster_name,cluster_group_name,M,S,TS,LS,OS,server_type
1,prod_cluster_full,group_a,172.27.17.25,172.27.21.156,172.27.21.158,,172.27.21.157,vm_l
```

## 模板生成规则

脚本不再从 CSV 中读取模板列，而是根据 `server_type` 直接扩展模板名称。

给定：

```text
server_type = {x}
```

则自动生成：

- `global_template = template_{x}_cluster`
- `dn_template = template_{x}_dn`
- `cn_template = template_{x}_cn`
- `os_cn_template = template_{x}_cn_OS`

例如：

- `server_type=vm_l`
  - `template_vm_l_cluster`
  - `template_vm_l_dn`
  - `template_vm_l_cn`
  - `template_vm_l_cn_OS`
- `server_type=vm_lowercase_0`
  - `template_vm_lowercase_0_cluster`
  - `template_vm_lowercase_0_dn`
  - `template_vm_lowercase_0_cn`
  - `template_vm_lowercase_0_cn_OS`

## 模板使用规则

顶层 `parameterTemplateInfos` 固定生成三类模板：

- `GLOBAL`
- `DN`
- `CN`

即：

```json
[
  {"type": "DN", "templateName": "template_{server_type}_dn"},
  {"type": "CN", "templateName": "template_{server_type}_cn"},
  {"type": "GLOBAL", "templateName": "template_{server_type}_cluster"}
]
```

`OS` 角色上的 CN 使用特殊模板：

- 若存在 `OS` 角色，则 `OS` 角色上的 CN 节点级模板为 `template_{server_type}_cn_OS`
- 若不存在 `OS` 角色，则不使用 `os_cn_template`，也不会在模板选择展示中输出该项

其他角色上的 CN 均使用普通 `cn_template`。

## 节点生成规则

### CN

每个非空角色 IP 生成两个 CN 实例：

- `dbproxy1`，`servicePort=3306`
- `dbproxy2`，`servicePort=3307`

默认安装用户前缀为 `nu`，因此默认用户名为：

- `nudbproxy1`
- `nudbproxy2`

### DN

每个非空角色 IP 生成一个 DN 实例，安装用户固定为：

- `{prefix}db1`

默认前缀为 `nu`，因此默认用户名为：

- `nudb1`

DN 角色映射：

- `M -> dbRole=1`
- `S -> dbRole=0`
- `TS -> dbRole=0`
- `LS -> dbRole=0`
- `OS -> dbRole=2`

DN teamId 映射：

- `M -> teamId=1`
- `S -> teamId=2`
- `LS -> teamId=3`
- `OS -> teamId=4`
- `TS -> teamId=5`

## 执行前模板展示

脚本在实际执行前会输出每个集群最终选择的模板名称。

若存在 `OS` 角色，展示：

```text
cluster=prod_cluster_full template_selection={
  "server_type":"vm_l",
  "global_template":"template_vm_l_cluster",
  "dn_template":"template_vm_l_dn",
  "cn_template":"template_vm_l_cn",
  "os_cn_template":"template_vm_l_cn_OS"
}
```

若不存在 `OS` 角色，则不展示 `os_cn_template`。

## 常用参数

| 参数 | 说明 |
| --- | --- |
| `--api` | Insight 地址，必填 |
| `--csv` | CSV 文件路径，必填 |
| `--prefix` | 安装用户名前缀，默认 `nu` |
| `--base-path` | 安装根目录，默认 `/data/goldendb` |
| `--ins-user-pwd` | 业务用户密码明文 |
| `--ins-user-pwd-base64` | 业务用户密码 base64 |
| `--wait-completion` | 提交后等待安装完成 |
| `--max-wait-time` | 最大等待秒数，默认 `3600` |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--no-verify` | 跳过 SSL 证书校验，默认开启 |
| `--verify-ssl` | 启用 SSL 证书校验 |
| `--dry-run` | 只渲染请求体，不发起接口调用 |
| `--format` | 输出格式：`json` 或 `text` |

## 返回结果

脚本标准输出默认为 JSON，包含：

- `summary`
- `clusters`
- 每个集群的 `template_selection`
- 每个集群的 `request_payload`
- 执行结果或错误信息

创建成功时，从接口返回中读取：

```json
{
  "code": 1,
  "msg": "success",
  "data": {
    "task_id": 1
  }
}
```

进度查询按以下状态判断：

- `result=running`：继续轮询
- `result=success`：创建成功
- `result=fail`：创建失败，脚本返回错误并保留 `errorCode/errorMsg`
