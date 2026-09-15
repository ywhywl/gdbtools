# insight-batch-create 设计与差异版本更新文档

本文合并了原 `insight_batch_create_design.md` 和
`insight_batch_create_port_logic_diff.md` 的设计、端口、模板及扩展节点规则，记录多 IP 集群扩展的最终实现要求。

当前版本已经实施代码调整；本文用于说明新旧行为差异和请求体生成规则。

## 一、变更范围

涉及文件：

```text
internal/insightbatchcreate/run.go
internal/insightbatchcreate/template_resolver.go
internal/insightbatchcreate/run_test.go
cmd/insight-batch-create/main.go
```

`main.go` 仍然只负责调用 `insightbatchcreate.Run()`，本次不需要增加入口逻辑。

文档文件：

```text
docs/insight_batch_create.md
docs/insight_batch_create_design.md
docs/insight_batch_create_port_logic_diff.md
docs/insight_batch_create_design_diff.md
```

## 二、多 IP 输入规则

所有角色字段均按从左到右的顺序展开 IP。

`;` 和 `|` 是完全等价的多 IP 分隔符，可以混用：

```text
10.0.0.1;10.0.0.2|10.0.0.3
```

等价于：

```text
10.0.0.1|10.0.0.2|10.0.0.3
```

角色支持情况：

| 角色 | 多 IP | 规则 |
| --- | --- | --- |
| M | 否 | 只能配置一个 IP |
| S | 是 | 支持 `;`、`|` |
| LS | 是 | 支持 `;`、`|` |
| OS | 是 | 支持 `;`、`|` |
| TS | 是 | 支持 `;`、`|` |

OS 不再把 `;` 解释为 IDC 分组。它和 `|` 具有相同含义，仅表示下一个 IP。

以下输入均合法：

```text
OS=10.0.0.1
OS=10.0.0.1|10.0.0.2
OS=10.0.0.1;10.0.0.2
OS=10.0.0.1|10.0.0.2;10.0.0.3|10.0.0.4
```

## 三、输入校验

### 3.1 必填和 M 角色

- `M`、`S` 不能为空。
- `M` 只能有一个 IP，包含 `;` 或 `|` 时直接报错。
- `TS`、`LS`、`OS` 可以为空，也可以配置多个 IP。

### 3.2 空元素

多 IP 列表中出现空元素时直接报错，不会静默忽略：

```text
10.0.0.1||10.0.0.2
10.0.0.1;;10.0.0.2
10.0.0.1;|10.0.0.2
|10.0.0.1
10.0.0.1|
10.0.0.1; ;10.0.0.2
```

### 3.3 全局 IP 唯一

同一集群的 `M`、`S`、`TS`、`LS`、`OS` 所有 IP 必须全局唯一，不能在同一角色内或不同角色间重复。

校验发生在 API 调用前，错误信息包含 CSV 行号、重复 IP 及涉及角色。

## 四、teamId 分配

### 4.1 基础 teamId

每个角色的第一个 IP 保持现有 teamId：

| 角色 | 第一个 IP 的 teamId |
| --- | ---: |
| M | 1 |
| S | 2 |
| LS | 3 |
| OS | 4 |
| TS | 5 |

### 4.2 扩展 teamId

每个 IP 独立生成一个 team。额外 IP 使用集群级统一计数器，从 `61` 开始递增：

```text
61、62、63、64、65……
```

计数器：

- 不按角色重置；
- 不按 IDC 重置；
- 不区分 `;` 或 `|`；
- 每个集群单独从 61 开始；
- 按角色顺序 `M -> S -> LS -> OS -> TS`，角色内按输入顺序分配。

例如：

```text
M=10.0.0.1
S=10.0.0.2|10.0.0.3
OS=10.0.0.4;10.0.0.5|10.0.0.6
TS=10.0.0.7|10.0.0.8
```

对应 teamId：

| 角色 | IP | teamId |
| --- | --- | ---: |
| M | 10.0.0.1 | 1 |
| S | 10.0.0.2 | 2 |
| S | 10.0.0.3 | 61 |
| OS | 10.0.0.4 | 4 |
| OS | 10.0.0.5 | 62 |
| OS | 10.0.0.6 | 63 |
| TS | 10.0.0.7 | 5 |
| TS | 10.0.0.8 | 64 |

## 五、OS 逻辑主

一个集群只能有一个 OS 逻辑主。

OS 展开后的第一个 IP 固定为唯一逻辑主：

```text
OS=10.0.0.1|10.0.0.2;10.0.0.3
```

生成：

| IP | dbRole | teamId |
| --- | ---: | ---: |
| 10.0.0.1 | 2，逻辑主 | 4 |
| 10.0.0.2 | 0，普通备库 | 61 |
| 10.0.0.3 | 0，普通备库 | 62 |

后续 IP 不会因为使用 `;` 而重新成为逻辑主。

其他角色的 `dbRole` 保持原规则：

| 角色 | dbRole |
| --- | ---: |
| M | 1 |
| S | 0 |
| TS | 0 |
| LS | 0 |
| OS 第一个 IP | 2 |
| OS 其他 IP | 0 |

OS 和 TS 可以同时配置。

## 六、DN 请求体差异

旧行为是每个角色最多生成一个 team。新行为是：

```text
一个 IP -> 一个 team
一个 team -> 一个 dnList 节点
```

示例：

```json
{
  "dbgroupId": 1,
  "teamList": [
    {
      "teamId": 4,
      "dnList": [{
        "ip": "10.0.0.1",
        "dbRole": 2,
        "templateName": "template_vm_l_dn_OS.json"
      }]
    },
    {
      "teamId": 61,
      "dnList": [{
        "ip": "10.0.0.2",
        "dbRole": 0,
        "templateName": "template_vm_l_dn_OS.json"
      }]
    }
  ]
}
```

所有 OS DN 使用同一 OS 专用模板：

```text
template_{server_type}_dn_OS.json
```

普通 M/S/TS/LS DN 使用全局 DN 模板：

```text
template_{server_type}_dn.json
```

## 七、CN 端口和模板差异

每个 IP 都会按角色生成 CN。

| 角色 | CN servicePort | CN 数量 |
| --- | --- | ---: |
| M | 3306、3307 | 2 |
| S | 3306、3307 | 2 |
| TS | 3306、3307 | 2 |
| LS | 3308 | 1 |
| OS | 3309 | 1 |

CN 用户名规则保持不变：

```text
3306 -> {prefix}dbproxy1
3307 -> {prefix}dbproxy2
3308 -> {prefix}dbproxy1
3309 -> {prefix}dbproxy1
```

所有角色，包括 OS 和 LS，都使用统一的 CN 模板：

```text
template_{server_type}_cn.json
```

不使用 OS 专用 CN 模板。

## 八、模板自动选择差异

自动检测覆盖集群内全部 IP，包括多 IP 角色的每个主机。

### 8.1 内存映射

| 虚拟化类型 | 内存 MemGB | server_type |
| --- | ---: | --- |
| 物理机 | 任意 | `pm` |
| 虚拟机 | < 24 | 报错；仅在类型不一致且指定允许参数时使用 `vm_l` |
| 虚拟机 | >= 24 且 < 30 | `vm_l` |
| 虚拟机 | >= 30 且 < 46 | `vm_m` |
| 虚拟机 | >= 46 | `vm_h` |

虚拟机内存低于 24G 时默认按原业务逻辑因内存阈值报错退出。已有的 `--allow-low-memory-vm` 逻辑继续保留：指定后可降级使用 `vm_l`。此外，只有当一个集群包含多个节点、节点检测出的 `server_type` 不一致，并且指定 `--allow-server-type-mismatch` 时，才允许忽略该类型差异；如果不一致场景包含低内存虚拟机，则使用 `vm_l` 模板继续。

### 8.2 主机 server_type 一致性

默认要求同一集群所有主机检测到一致的 `server_type`：

```text
10.0.0.1 -> vm_l
10.0.0.2 -> vm_l
10.0.0.3 -> vm_m
```

默认报错，不提交创建请求。

使用：

```text
--allow-server-type-mismatch
```

启用后：

- 主机间 `server_type` 不一致只打印告警；
- 任一主机 SSH 连接失败仍然报错；
- 以实际 SSH 检测值优先，不采用 CSV 中冲突的 `server_type`；
- 使用按 IP 检测顺序的第一个实际 `server_type` 作为集群模板基准；
- 继续生成并提交请求。

如果不一致场景中包含内存低于 24G 的虚拟机，则使用 `vm_l` 模板。若存在低于 24G 的虚拟机但节点类型一致，`--allow-server-type-mismatch` 本身不会放宽内存阈值；此时仍需依靠原有的 `--allow-low-memory-vm` 逻辑。

`--allow-server-type-mismatch` 与 `--skip-template-check` 互斥，因为前者实际值优先，后者 CSV 值优先。

原有 `--ignore-template-mismatch` 仍用于 CSV 指定值与自动检测值的冲突；启用新参数时，以实际检测值为准。

## 九、执行流程

```text
1. 解析命令行参数
2. 创建 Insight 客户端
3. 读取 CSV
4. 按 ; 和 | 展开各角色 IP
5. 校验 M 单 IP、空元素和全局 IP 唯一性
6. 自动检测全部主机并选择 server_type
7. 校验主机 server_type 一致性，或按参数告警继续
8. 为所有 IP 分配 teamId
9. 构建 DN/CN 请求体
10. 提交 createCluster 请求
11. 可选轮询安装进度
12. 输出结果
```

## 十、测试要求

已增加或需要覆盖：

- 单 IP 输入兼容；
- `;`、`|` 及混合分隔符；
- M 多 IP报错；
- 空元素报错；
- 集群全局 IP 重复报错；
- S、TS、LS、OS 多 IP；
- OS 只有一个逻辑主；
- OS 所有节点使用 OS DN 模板；
- 每 IP 一个 team；
- 基础 teamId 保持 1～5；
- 扩展 teamId 跨角色从 61 连续递增；
- OS 与 TS 同时配置；
- 虚拟机内存低于 24G 默认报错；保留 `--allow-low-memory-vm` 原有降级逻辑；类型不一致且指定 `--allow-server-type-mismatch` 时也可使用 `vm_l`；
- 默认拒绝主机 server_type 不一致；
- `--allow-server-type-mismatch` 告警并使用实际检测值；若不一致包含低内存虚拟机，则使用 `vm_l`。

## 十一、历史差异总结

| 项目 | 原行为 | 新行为 |
| --- | --- | --- |
| 角色 IP | 每角色一个 IP | S/TS/LS/OS 支持多 IP，M 保持单 IP |
| 分隔符 | 无多 IP规则 | `;`、`|` 等价，可混用 |
| OS IDC | 可被解释为分组 | 不保留 IDC 语义 |
| team | 每角色最多一个 | 每 IP 一个 team |
| 基础 teamId | 1～5 | 保持不变 |
| 扩展 teamId | 不支持 | 集群级从 61 递增 |
| OS 逻辑主 | 单 IP时为 teamId=4 | 展开后第一个 IP唯一为 teamId=4 |
| OS DN 模板 | 节点级模板未完整传入 | 所有 OS DN 显式使用 OS 模板 |
| 主机类型不一致 | 直接报错 | 默认报错，可用 `--allow-server-type-mismatch` 告警继续 |
| 低内存 VM | 低于阈值报错 | 保留 `--allow-low-memory-vm` 原逻辑；类型不一致且指定 `--allow-server-type-mismatch` 时也可使用 `vm_l` |
| OS CN 模板 | 曾存在专用模板设计 | 所有角色统一使用 CN 模板 |
