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
| `--insight-user` | Insight 登录用户名，必填 |
| `--insight-password` | Insight 登录密码明文 |
| `--insight-password-b64` | Insight 登录密码 base64 |
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
| `--debug` | 打印请求和响应的 debug 日志，默认关闭 |
| `--skip-check` | 跳过主机前置检查，默认开启检查 |
| `--check-timeout` | 单台主机检查超时秒数，默认 `15` |

注意：

- 必须提供 Insight 鉴权参数，鉴权头为 `username/password`
- 如果未显式提供 `--ssh-password` 且未修改 `--ssh-password-b64`，命令会使用默认测试值 `c2VjcmV0`

## 前置检查

命令默认在纳管前通过 SSH 检查每台主机，未通过检查的主机会被跳过。

### 检查规则

| 检查项 | 物理机 | 虚拟机 |
| --- | --- | --- |
| 操作系统 | 必须为麒麟（kylin/neokylin），CentOS 不通过 | 必须为麒麟（kylin/neokylin），CentOS 不通过 |
| CPU 架构 | 记录（aarch64 / x86\_64） | 记录（aarch64 / x86\_64） |
| /data 挂载 | 必须挂载 | 不能挂载 |
| /data 可用空间 | ≥ 3072 GB (3T) | — |
| CPU 核心数 | ≥ 50 | < 20 |
| 内存 | ≥ 200 GB | 24 GB ~ 48 GB |

### 检查输出

```
[check] 检查主机: 10.0.0.21
[check] PASS 10.0.0.21: os=kylin arch=aarch64 virt=none cpu=64 mem=256G data_mount=true data_avail=4096G
[check] FAIL 10.0.0.22: os=centos arch=x86_64 virt=kvm cpu=4 mem=8G data_mount=false data_avail=0G
前置检查完成: 通过 1, 未通过 1
进入纳管的主机: 1 台
```

## 使用示例

```bash
go run ./cmd/insight-batch-onboard-hosts \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
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
