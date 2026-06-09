# insight-batch-onboard-hosts 使用说明

## 功能说明

`insight-batch-onboard-hosts` 用于批量调用 Insight 主机纳管接口，将主机分批纳管到平台。

接口：

- 纳管：`/open_api/insight/external/host/batchAddhost`
- 查询进度：`/open_api/insight/external/host/querybatchAddHostResult`

命令入口：

```bash
go run ./cmd/insight-batch-onboard-hosts --help
```

## 输入格式

支持 `CSV` 和 `JSON`。

CSV 常用字段：

| 字段名 | 是否必填 | 说明 |
| --- | --- | --- |
| `room_name` | 否 | 机房名称 |
| `server_ip` | 是 | 主机 IP |
| `install_path` | 否 | 安装路径，默认 `/` |
| `data_path` | 否 | 数据路径，默认 `/` |
| `system_parameter` | 否 | 系统参数，默认 `1` |
| `region` | 否 | 作为 `label` 使用 |

CSV 示例：

```csv
room_name,server_ip,install_path,data_path,system_parameter,region
room_a,10.0.0.21,/data/gdb,/data/gdb/data,1,shanghai
room_a,10.0.0.22,/data/gdb,/data/gdb/data,1,shanghai
```

JSON 示例：

```json
[
  {
    "room_name": "room_a",
    "server_ip": "10.0.0.21",
    "install_path": "/data/gdb",
    "data_path": "/data/gdb/data",
    "system_parameter": "1",
    "region": "shanghai"
  }
]
```

## 参数说明

| 参数 | 说明 |
| --- | --- |
| `--api` | Insight 地址，必填 |
| `--input` | 输入文件路径，必填 |
| `--ssh-port` | SSH 端口，默认 `22` |
| `--ssh-user` | SSH 用户名，必填 |
| `--ssh-password` | SSH 密码明文 |
| `--ssh-password-b64` | SSH 密码 base64，默认 `c2VjcmV0` |
| `--cover-install` | 是否覆盖安装，`0` 或 `1`，默认 `0` |
| `--batch-size` | 每批纳管主机数，范围 `1-10`，默认 `10` |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--poll-timeout` | 轮询超时秒数，默认 `3600` |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |
| `--output-json` | 以 JSON 输出结果 |

注意：

- 如果未显式提供 `--ssh-password` 且未修改 `--ssh-password-b64`，命令会使用默认测试值 `c2VjcmV0`

## 使用示例

```bash
go run ./cmd/insight-batch-onboard-hosts \
  --api 10.0.0.10:8444 \
  --input ./hosts.csv \
  --ssh-user root \
  --ssh-password 'plain-password' \
  --batch-size 5 \
  --output-json
```

## 输出结果

启用 `--output-json` 时输出：

- `total`
- `success_count`
- `failed_count`
- `results`

其中 `results` 格式为：

```json
[
  {"ip": "10.0.0.21", "status": "success"},
  {"ip": "10.0.0.22", "status": "failed"}
]
```

未启用 `--output-json` 时，输出日志摘要。

## 返回码

- `0`：全部成功
- `1`：存在失败主机，或输入为空
- `2`：参数错误或请求失败
