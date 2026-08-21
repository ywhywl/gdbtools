# insight-batch-create 设计文档

## 目标

`insight-batch-create` 用于根据 CSV 清单批量调用 GoldenDB Insight 的新增租户接口。支持：

- 自动通过 SSH 检测主机内存和虚拟化类型，自动选择模版
- CSV 指定 server_type 与自动检测结果的比对和冲突处理
- 提交后轮询安装进度
- Dry-run 模式只渲染请求体

## 架构

### 接口调用

| 接口 | 方法 | 说明 |
|------|------|------|
| `/open_api/insight/external/tenant/createCluster` | POST | 创建租户集群 |
| `/open_api/insight/external/tenant/getInstallClusterProcess` | GET | 查询安装进度 |

### 目录结构

```
cmd/insight-batch-create/
  main.go              # 入口，调用 Run()

internal/insightbatchcreate/
  run.go               # 参数解析、流程编排、API 调用
  template_resolver.go # SSH 检测主机内存和虚拟化，选择 server_type
```

### 执行流程

```
1. 解析参数 (parseArgs)
2. 创建 Insight 客户端
3. 解析 CSV (loadRows)
4. 模版自动选择 (applyAutoSelect) [可选]
   4.1 收集各集群的唯一 IP 列表
   4.2 SSH 检测每台主机的内存和虚拟化
   4.3 根据内存阈值确定 server_type
   4.4 校验同集群所有 IP 的 server_type 一致
   4.5 处理 CSV 指定值与自动检测的冲突
5. 为每个集群构建请求体 (buildPayload)
6. 提交创建请求，失败重试 (executeRow)
7. 可选轮询安装进度 (pollCreateClusterProgress)
8. 输出结果
```

## SSH 认证

SSH 认证用于前置检测和模版自动选择，认证优先级：

1. **SSH Agent** — 如果 `$SSH_AUTH_SOCK` 可用
2. **私钥文件** — 自动查找 `~/.ssh/id_ed25519` → `id_rsa` → `id_ecdsa` → `id_dsa`，或通过 `--ssh-key` 指定
3. **密码** — `--ssh-password` 或 `--ssh-password-b64`，key 认证失败时的兜底

有 key 时不需要提供密码。如果无任何认证方式可用，自动选择模版会报错。

## 模版自动选择

### 虚拟化检测

使用 DMI + CPU hypervisor flag 双通道检测：

1. **DMI product name** — `cat /sys/class/dmi/id/product_name`
   - 包含虚拟化关键词（kvm/qemu/vmware/xen/hyper-v/cvm/ecs/bcc 等）→ VM
   - 包含物理机品牌词（dell/lenovo/inspur/huawei 等）→ 物理机
2. **CPU hypervisor flag** — `grep -qw hypervisor /proc/cpuinfo`
   - 存在 → VM（x86 铁证）
   - 不存在 → 不影响判断（aarch64 无此 flag）

### 内存 → server_type 映射

| 虚拟化类型 | 内存 MemGB (free -g) | server_type |
|-----------|---------------------|-------------|
| 物理机 | 任意 | `pm` |
| 虚拟机 | < 23 | **报错退出** |
| 虚拟机 | >= 23 且 < 30 | `vm_l` |
| 虚拟机 | >= 30 且 < 46 | `vm_m` |
| 虚拟机 | >= 46 | `vm_h` |

阈值已考虑 `free -g` 向下取整的偏差（32G 物理机通常显示 31G）。

### 冲突处理

| `--ignore-template-mismatch` | `--skip-template-check` | 行为 |
|------------------------------|-------------------------|------|
| 未指定 | 未指定 | 不一致则报错退出 |
| 指定 | 未指定 | 使用自动检测结果 |
| 未指定 | 指定 | 使用 CSV 指定值 |
| 指定 | 指定 | 互斥，报错退出 |

## 请求体构建

### parameterTemplateInfos（全局模版）

只发送三种类型：

```json
"parameterTemplateInfos": [
  {"type": "DN", "templateName": "template_{type}_dn.json"},
  {"type": "CN", "templateName": "template_{type}_cn.json"},
  {"type": "GLOBAL", "templateName": "template_{type}_cluster.json"}
]
```

### DnOSTemplate（节点级模版）

OS 角色的 DN 节点使用独立模版，在 `dnInstallList` 的 `templateName` 字段设置：

```json
"dnInstallList": [{
  "dbgroupId": 1,
  "teamList": [{
    "teamId": 4,
    "dnList": [{
      "ip": "10.0.0.5",
      "dbRole": 2,
      "templateName": "template_{type}_dn_OS.json"
    }]
  }]
}]
```

节点级模版会覆盖全局 DN 模版。

## 返回码

| 返回码 | 含义 |
|--------|------|
| 0 | 全部成功，或 dry-run |
| 1 | 部分成功、部分失败 |
| 2 | 全部失败 |
| 3 | 参数错误或执行前校验失败 |
