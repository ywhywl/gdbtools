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
| `system_parameter` | 否 | 系统参数，默认 `1` |
| `region` | 否 | 作为 `label` 使用 |

CSV 示例：

```csv
room_name,server_ip,system_parameter,region
room_a,10.0.0.21,1,shanghai
room_a,10.0.0.22,1,shanghai
```

JSON 示例：

```json
[
  {
    "room_name": "room_a",
    "server_ip": "10.0.0.21",
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
| `--ssh-key` | SSH 私钥路径，不指定则自动查找 `~/.ssh/id_ed25519` → `id_rsa` → `id_ecdsa` → `id_dsa` |
| `--ssh-password` | SSH 密码明文（key 认证失败时的兜底） |
| `--ssh-password-b64` | SSH 密码 base64 |
| `--cover-install` | 是否覆盖安装，`0` 或 `1`，默认 `0` |
| `--batch-size` | 每批纳管主机数，范围 `1-10`，默认 `10` |
| `--poll-interval` | 轮询间隔秒数，默认 `10` |
| `--poll-timeout` | 轮询超时秒数，默认 `3600` |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |
| `--output-json` | 以 JSON 输出结果 |
| `--debug` | 打印请求和响应的 debug 日志，默认关闭 |
| `--skip-check` | 跳过主机前置检查，默认开启检查 |
| `--check-timeout` | 单台主机检查超时秒数，默认 `15` |
| `--required-os-version` | 要求的麒麟操作系统版本，默认 `V10` |

注意：

- 必须提供 Insight 鉴权参数，鉴权头为 `username/password`
- SSH 认证优先级：SSH Agent > 私钥文件 > 密码，有 key 时不需要提供密码参数

## 前置检查

命令默认在纳管前通过 SSH 检查每台主机，未通过检查的主机会被跳过。

### 路径自动确定

`data_path` 和 `install_path` **不再需要在 CSV/JSON 中指定**，由前置检查自动确定：

- 通过 `df -BG /data` 判断 `/data` 是否为真实挂载点（普通创建的目录不算）
- 有 `/data` 挂载 → `data_path=/data`, `install_path=/data`
- 无 `/data` 挂载 → `data_path=/`, `install_path=/`
- 即使使用 `--skip-check` 跳过完整检查，也会执行轻量的挂载检测来确定路径

### 检查规则

| 检查项 | 物理机 | 虚拟机 |
| --- | --- | --- |
| 操作系统 | 必须为麒麟 V10（可通过 `--required-os-version` 配置版本） | 必须为麒麟 V10（可通过 `--required-os-version` 配置版本） |
| CPU 类型 | 必须为海光（非 71xx 系列）或鲲鹏，不支持 Intel/AMD | 必须为海光（非 71xx 系列）或鲲鹏，不支持 Intel/AMD |
| CPU 架构 | 记录（aarch64 / x86\_64） | 记录（aarch64 / x86\_64） |
| /data 挂载 | 必须挂载 | 不能挂载 |
| /data 可用空间 | ≥ 3072 GB (3T) | — |
| CPU 核心数 | ≥ 50 | ≤ 30 |
| 内存 | ≥ 200 GB | 24 GB ~ 48 GB |

**CPU 类型说明**：
- **海光（Hygon）**：允许除 71xx 系列外的所有型号（如 C86 7185、C86 7280 等）
- **鲲鹏（Kunpeng）**：允许所有型号（如 Kunpeng-920）
- **海光 71xx 系列**：不允许（如 C86 7151、C86 7171 等）
- **Intel/AMD**：不允许

### 检查输出

```
[check] 检查主机: 10.0.0.21
[check] PASS 10.0.0.21: os=kylin/V10 cpu_vendor=hygon cpu_model=Hygon C86 7185 32-core Processor arch=x86_64 virt=none cpu=64 mem=256G data_mount=true data_avail=4096G
[check] 检查主机: 10.0.0.22
[check] FAIL 10.0.0.22: os=kylin/V10 cpu_vendor=intel cpu_model=Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz arch=x86_64 virt=kvm cpu=4 mem=8G data_mount=false data_avail=0G
[check]   原因: CPU 类型不支持: 检测到 Intel CPU (Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz)，仅支持海光（非71xx系列）或鲲鹏
[check] 检查主机: 10.0.0.23
[check] FAIL 10.0.0.23: os=kylin/V10 cpu_vendor=hygon cpu_model=Hygon C86 7151 16-core Processor arch=x86_64 virt=none cpu=32 mem=128G data_mount=true data_avail=2048G
[check]   原因: 海光 CPU 71xx 系列不支持: Hygon C86 7151 16-core Processor
前置检查完成: 通过 1, 未通过 2
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

- `total`：纳管主机总数
- `success_count`：成功数量
- `failed_count`：失败数量
- `results`：纳管结果列表
- `precheck`：前置检查详情（如果执行了检查）

`results` 格式：

```json
[
  {"ip": "10.0.0.21", "status": "success"},
  {"ip": "10.0.0.22", "status": "failed"}
]
```

`precheck` 格式（包含新增的 CPU 和 OS 版本字段）：

```json
[
  {
    "ip": "10.0.0.21",
    "passed": true,
    "os": "kylin",
    "os_version": "V10",
    "arch": "x86_64",
    "cpu_vendor": "hygon",
    "cpu_model": "Hygon C86 7185 32-core Processor",
    "type": "physical",
    "cpu": 64,
    "mem_gb": 256
  },
  {
    "ip": "10.0.0.22",
    "passed": false,
    "os": "kylin",
    "os_version": "V10",
    "arch": "x86_64",
    "cpu_vendor": "intel",
    "cpu_model": "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz",
    "type": "virtual",
    "cpu": 4,
    "mem_gb": 8,
    "reasons": [
      "CPU 类型不支持: 检测到 Intel CPU (Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz)，仅支持海光（非71xx系列）或鲲鹏"
    ]
  }
]
```

未启用 `--output-json` 时，输出日志摘要。

## 返回码

- `0`：全部成功
- `1`：存在失败主机，或输入为空
- `2`：参数错误或请求失败
