# insight-batch-add-cn 使用说明

## 功能说明

`insight-batch-add-cn` 用于批量向已有 GoldenDB 集群新增 CN 组件。

执行流程：

1. 解析输入文件
2. 按 `insight_addr + cluster_name + template_name` 分组
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
| `template_name` | 是 | CN 模板名 |
| `ip` | 是 | CN 所在主机 IP |

可选字段：

| 字段名 | 说明 |
| --- | --- |
| `port` | CN 端口 |
| `install_user` | 安装用户 |
| `install_path` | 安装路径 |
| `service_port` | 服务端口 |

CSV 示例：

```csv
insight_addr,cluster_name,template_name,ip,port,install_user,install_path,service_port
10.0.0.10:8444,prod_cluster_a,template_vm_l_cn,10.0.0.31,0,nudbproxy1,/data/goldendb/nudbproxy1,3306
10.0.0.10:8444,prod_cluster_a,template_vm_l_cn,10.0.0.32,0,nudbproxy2,/data/goldendb/nudbproxy2,3307
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
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--poll-timeout` | 轮询超时秒数，默认 `3600` |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |
| `--output-json` | 输出 JSON |

如果某行未提供 `port/install_user/install_path/service_port`，会使用对应的默认参数填充。

鉴权说明：

- 请求头统一带 `username/password`
- `password` 取 Insight 登录密码的 base64

## 使用示例

```bash
go run ./cmd/insight-batch-add-cn \
  --input ./batch_add_cn.csv \
  --insight-user admin \
  --insight-password 'insight-password' \
  --default-port 0 \
  --poll-interval 5 \
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
- `ip`
- `port`
- `status`
- `message`

## 返回码

- `0`：命令执行完成
- `2`：参数错误、输入校验失败或接口调用失败
