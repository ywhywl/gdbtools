# insight-batch-onboard-hosts 设计文档

## 目标

`insight-batch-onboard-hosts` 用于批量将主机纳管到 GoldenDB Insight 平台。支持：

- 前置检查：通过 SSH 检查主机操作系统、CPU、内存、/data 挂载等
- 路径自动确定：根据 /data 挂载情况自动设置 install_path 和 data_path
- 批量纳管，支持分批提交和轮询进度
- CSV/JSON 输入

## 架构

### 接口调用

| 接口 | 方法 | 说明 |
|------|------|------|
| `/open_api/insight/external/host/batchAddhost` | POST | 批量纳管主机 |
| `/open_api/insight/external/host/querybatchAddHostResult` | GET | 查询纳管进度 |

### 目录结构

```
cmd/insight-batch-onboard-hosts/
  main.go              # 入口，调用 Run()

internal/insightonboard/
  run.go               # 参数解析、流程编排、API 调用

internal/hostchecker/  # 共享主机检查模块
  check.go             # 前置检查逻辑
  client.go            # SSH 客户端（支持 key/agent/password 认证）
  types.go             # 数据结构
```

### 执行流程

```
1. 解析参数
2. 创建 Insight 客户端
3. 解析 CSV/JSON 输入
4. SSH 认证解析 (NewSSHAuth)
5. 前置检查 [默认开启]
   5.1 SSH 连接每台主机
   5.2 收集系统信息（OS/CPU/内存/挂载/虚拟化）
   5.3 检查规则判定
   5.4 自动确定 data_path/install_path
   5.5 未通过的主机被跳过
6. 跳过检查 (--skip-check)
   6.1 轻量 SSH mount 检测
   6.2 SSH 失败默认 /，成功且有 /data 挂载则用 /data
7. 分批纳管 (onboardHosts)
8. 轮询进度
9. 输出结果
```

## SSH 认证

纳管过程需要两种 SSH 认证：

### CLI 前置检查

用于我们自己的 CLI 登录目标主机执行检查命令：

1. **SSH Agent** — `$SSH_AUTH_SOCK`
2. **私钥文件** — 自动发现或 `--ssh-key` 指定
3. **密码** — `--ssh-password` 兜底

有 key 时不需要密码。如果无任何认证，前置检查跳过所有主机。

### Insight API 纳管密码

Insight 后端需要密码来 SSH 登录目标主机执行安装：

- `--ssh-password` 或 `--ssh-password-b64`
- **必须提供**，不传则报错

## 路径自动确定

`data_path` 和 `install_path` 不再需要在 CSV 中指定，由前置检查自动确定：

| 场景 | 确定方式 |
|------|---------|
| 物理机，/data 已挂载 | `/data` |
| 虚拟机，无 /data | `/` |
| `--skip-check`，SSH 成功且 /data 挂载 | `/data` |
| `--skip-check`，SSH 失败或无 /data | `/` |

检测通过 `df -BG /data` 判断，只认真实挂载点（创建的普通目录不算）。

## 前置检查规则

| 检查项 | 物理机 | 虚拟机 |
|--------|--------|--------|
| 操作系统 | 必须为麒麟 | 必须为麒麟 |
| /data 挂载 | 必须挂载 | 不能挂载 |
| /data 可用空间 | ≥ 3008G（3072G - 64G 容差） | — |
| CPU 核心数 | ≥ 50 | < 30 |
| 内存 | ≥ 196G（200G - 4G 容差） | 24G ~ 52G |

容差用于补偿 `free -g` 和 `df -BG` 的向下取整误差。

## 返回码

| 返回码 | 含义 |
|--------|------|
| 0 | 全部成功 |
| 1 | 存在失败主机 |
| 2 | 参数错误或请求失败 |
