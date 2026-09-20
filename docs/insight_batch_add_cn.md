# insight-batch-add-cn 使用说明

## 功能说明

`insight-batch-add-cn` 用于批量向已有 GoldenDB 集群新增 CN 组件。

执行流程：

1. 解析输入文件
2. 按 `insight_addr + cluster_name` 分组
3. 查询每组的 `clusterId`
4. 调用 `/open_api/insight/external/install/batchAddCN`
5. 轮询 `/open_api/insight/external/install/querybatchAddCNResult`
6. 汇总每行结果

命令入口：

```bash
go run ./cmd/insight-batch-add-cn --help
```

## 输入格式

支持 `CSV` 和 `JSON`。

必填字段：

| 字段名 | 是否必填 | 说明 |
| --- | --- | --- |
| `insight_addr` | 是 | Insight 地址 |
| `cluster_name` | 是 | 集群名称 |
| `role` | 使用自动端口时必填 | `M`、`S`、`TS`、`LS`、`OS`，用于推导 CN 服务端口 |
| `ip` | 是 | CN 所在主机 IP |

可选字段：

| 字段名 | 说明 |
| --- | --- |
| `port` | CN 端口 |
| `install_user` | 安装用户 |
| `install_path` | 安装路径 |
| `service_port` | 服务端口，可用 `3306;3307` 或 `3306|3307` 指定多个 |

CSV 示例：

```csv
insight_addr,cluster_name,role,ip,port,install_user,install_path,service_port
10.0.0.10:8444,prod_cluster_a,M,10.0.0.31,,,,3306;3307
10.0.0.10:8444,prod_cluster_a,M,10.0.0.32,,,,
```

## 参数说明

| 参数 | 说明 |
| --- | --- |
| `--input` | 输入文件路径，必填 |
| `--insight-user` | Insight 登录用户名，必填 |
| `--insight-password` | Insight 登录密码明文 |
| `--insight-password-b64` | Insight 登录密码 base64 |
| `--default-port` | 默认 CN 端口 |
| `--default-install-user` | 默认安装用户 |
| `--default-install-path` | 默认安装路径 |
| `--default-service-port` | 默认服务端口 |
| `--prefix` | 自动生成安装用户名的前缀，默认 `nu` |
| `--base-path` | 自动生成安装路径的根目录，默认 `/data/goldendb` |
| `--allow-role-port-mismatch` | 允许人工 service_port 与 role 不匹配 |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--poll-timeout` | 轮询超时秒数，默认 `3600` |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |
| `--output-json` | 输出 JSON |
| `--debug` | 打印实际请求 URL、请求体和响应的 debug 日志，默认关闭 |

规则：

- CN 接口已废弃 `parameterTemplateInfos`，不再填写或发送 `template_name/server_type`；历史输入中的这两个字段会被忽略。
- 未填写 `service_port` 时，按 role 推导：M/S/TS 为 `3306;3307`，LS 为 `3308`，OS 为 `3309`。
- 填写 `service_port` 后只生成指定端口；支持 `;` 或 `|` 分隔，不支持逗号。
- 标准端口自动生成安装用户名：3306/3308/3309 对应 `{prefix}dbproxy1`，3307 对应 `{prefix}dbproxy2`。
- 非标准端口必须人工填写 `install_user`；`--allow-role-port-mismatch` 只放宽角色与端口校验，不取消该要求。
- `install_path` 未填写时自动使用 `{base-path}/{install_user}`。行内值优先于命令行默认值和自动规则。

鉴权说明：

- 请求头统一带 `username/password`
- `password` 取 Insight 登录密码的 base64

## 使用示例

```bash
go run ./cmd/insight-batch-add-cn \
  --input ./batch_add_cn.csv \
  --insight-user admin \
  --insight-password 'insight-password' \
  --base-path /data/goldendb \
  --poll-interval 5 \
  --debug \
  --output-json
```

安装进度默认输出到 stderr；`--debug` 额外输出实际请求 URL、请求体和响应体。CN 请求体不再包含 `parameterTemplateInfos`，结构类似：

```json
{
  "clusterId": 1,
  "cnList": [{"ip": "10.0.0.31", "servicePort": 3306}]
}
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
- `ip`
- `port`
- `status`
- `message`

## 返回码

- `0`：命令执行完成
- `2`：参数错误、输入校验失败或接口调用失败
