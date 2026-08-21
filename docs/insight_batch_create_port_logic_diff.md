# insight-batch-create 端口和模版逻辑差异分析

## 问题描述

当前 Go 实现的 `insight-batch-create` 与历史 Python 实现在 LS/OS 角色的端口分配和模版选择上存在逻辑差异。

## 正确的端口分配逻辑（已确认）

### 应该实现的逻辑
- **M, S, TS 角色**：CN 端口为 3306 和 3307（nudbproxy1, nudbproxy2）
- **LS 角色**：CN 端口**仅** 3308（nudbproxy1）
- **OS 角色**：CN 端口**仅** 3309（nudbproxy1）

### 用户名规则
- 所有角色的 CN 用户名都从 `nudbproxy1` 开始
- M, S, TS：创建 `nudbproxy1` (3306) 和 `nudbproxy2` (3307)
- LS：创建 `nudbproxy1` (3308)
- OS：创建 `nudbproxy1` (3309)

## 历史实现对比

### Python 版本 (scripts/batch_create_from_csv.py)

**端口逻辑** (第179-197行):
```python
def build_cn_install_list(row: NormalizedRow, args: argparse.Namespace) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    for role in ROLE_SEQUENCE:
        ip = row.role_ips[role]
        if not ip:
            continue
        template_name = row.templates.os_cn_template if role == "OS" else row.templates.cn_template
        for suffix, service_port in ((1, 3306), (2, 3307)):  # ❌ 所有角色都是 3306 和 3307
            install_user = f"{args.prefix}dbproxy{suffix}"
            item: dict[str, Any] = {
                "ip": ip,
                "installPath": f"{args.base_path}/{install_user}",
                "installUser": install_user,
                "servicePort": service_port,
            }
            if role == "OS":
                item["templateName"] = template_name
            items.append(item)
    return items
```

**模版逻辑** (第117-128行):
```python
def resolve_templates(server_type: str) -> TemplateSelection:
    return TemplateSelection(
        server_type=normalized,
        global_template=f"template_{normalized}_cluster",
        dn_template=f"template_{normalized}_dn",
        cn_template=f"template_{normalized}_cn",
        os_cn_template=f"template_{normalized}_cn_OS",  # ✅ OS 专用 CN 模版
    )
```

### 当前 Go 实现 (internal/insightbatchcreate/run.go)

**端口逻辑** (第520-542行):
```go
func buildCNInstallList(row normalizedRow, args runArgs) []map[string]any {
    items := []map[string]any{}
    for _, role := range roleSequence {
        ip := row.RoleIPs[role]
        if ip == "" {
            continue
        }
        for _, item := range []struct {
            Suffix      int
            ServicePort int
        }{{1, 3306}, {2, 3307}} {  // ❌ 所有角色都是 3306 和 3307
            installUser := fmt.Sprintf("%sdbproxy%d", args.Prefix, item.Suffix)
            items = append(items, map[string]any{
                "ip":          ip,
                "installPath": fmt.Sprintf("%s/%s", args.BasePath, installUser),
                "installUser": installUser,
                "servicePort": item.ServicePort,
            })
        }
    }
    return items
}
```

**模版逻辑** (第463-488行):
```go
func resolveTemplates(serverType string, caseSensitive bool) (templateSelection, error) {
    // ...
    return templateSelection{
        ServerType:      normalized,
        GlobalTemplate:  fmt.Sprintf("template_%s_cluster.json", normalized+suffix),
        DNTemplate:      fmt.Sprintf("template_%s_dn.json", normalized+suffix),
        CNTemplate:      fmt.Sprintf("template_%s_cn.json", normalized+suffix),
        ClusterTemplate: fmt.Sprintf("template_%s_cluster.json", normalized+suffix),
        GTMTemplate:     fmt.Sprintf("template_%s_gtm.json", normalized+suffix),
        LDSTemplate:     fmt.Sprintf("template_%s_lds.json", normalized+suffix),
        SystemTemplate:  fmt.Sprintf("template_%s_system.json", normalized+suffix),
        DnOSTemplate:    fmt.Sprintf("template_%s_dn_OS.json", normalized+suffix),  // ✅ OS 专用 DN 模版
        // ❌ 缺少 OS 专用 CN 模版
    }, nil
}
```

## 需要修复的差异

### 1. 端口分配差异

**当前问题**：
- Python 和 Go 实现都对所有角色使用 3306 和 3307 端口

**应该实现**（根据用户确认的需求）：
```go
func buildCNInstallList(row normalizedRow, args runArgs) []map[string]any {
    items := []map[string]any{}
    for _, role := range roleSequence {
        ip := row.RoleIPs[role]
        if ip == "" {
            continue
        }
        
        // 根据角色确定端口配置
        var ports []struct {
            Suffix      int
            ServicePort int
        }
        
        if role == "LS" {
            // LS 只有 3308 端口，用户名仍从 nudbproxy1 开始
            ports = []struct {
                Suffix      int
                ServicePort int
            }{{1, 3308}}
        } else if role == "OS" {
            // OS 只有 3309 端口，用户名仍从 nudbproxy1 开始
            ports = []struct {
                Suffix      int
                ServicePort int
            }{{1, 3309}}
        } else {
            // M, S, TS 有 3306 和 3307 端口
            ports = []struct {
                Suffix      int
                ServicePort int
            }{{1, 3306}, {2, 3307}}
        }
        
        // 为每个端口创建安装项
        for _, port := range ports {
            installUser := fmt.Sprintf("%sdbproxy%d", args.Prefix, port.Suffix)
            item := map[string]any{
                "ip":          ip,
                "installPath": fmt.Sprintf("%s/%s", args.BasePath, installUser),
                "installUser": installUser,
                "servicePort": port.ServicePort,
            }
            
            // CN 不需要专用模版，所有角色都使用统一的 CNTemplate
            // 无需添加 templateName 字段
            
            items = append(items, item)
        }
    }
    return items
}
```

### 2. 模版选择差异（已确认：CN 不需要专用模版）

**当前问题**：
- Python 实现有 `os_cn_template` 字段但**不应该存在**
- Go 实现的 `DnOSTemplate` 是正确的（对应 DN 的 OS 专用模版）

**正确逻辑**（已确认）：
- **CN 模版**：所有角色（包括 OS）都使用统一的 `template_{server_type}_cn.json`，**不需要** OS 专用 CN 模版
- **DN 模版**：OS 角色使用专用的 `template_{server_type}_dn_OS.json`（当前 Go 实现的 `DnOSTemplate` 字段是正确的）

**结论**：
- Go 实现的模版结构是正确的，无需修改
- Python 实现的 `os_cn_template` 字段是错误的设计

## 待确认的问题（已全部确认）

**已确认信息**：

1. **LS 的 CN 模版**：✅ 不需要专用模版，使用通用的 `template_{server_type}_cn.json`

2. **用户名规则**：✅ 所有角色的 CN 用户名都从 `nudbproxy1` 开始
   - M/S/TS: `nudbproxy1` (3306), `nudbproxy2` (3307)
   - LS: `nudbproxy1` (3308)
   - OS: `nudbproxy1` (3309)

3. **端口分配**：✅ 
   - LS 只有 3308 端口
   - OS 只有 3309 端口（不是 3308）

4. **CN 模版**：✅ 所有角色统一使用 `template_{server_type}_cn.json`，不需要角色专用 CN 模版

5. **DN 模版**：✅ OS 角色使用专用的 `template_{server_type}_dn_OS.json`（当前 Go 实现已正确）

## 新发现的问题：OS 角色的 DN 模版未传入

### 问题描述

根据接口文档 `新增租户.txt`，`dnList` 中每个 DN 节点支持 `templateName` 字段：

```json
{
  "dnInstallList": [
    {
      "dbgroupId": 1,
      "teamList": [
        {
          "dnList": [
            {
              "ip": "127.0.0.1",
              "dbRole": 1,
              "installPath": "/home/goldendb",
              "installUser": "db1",
              "dataPath": "/home/goldendb",
              "templateName": "custom_dn2.json"  // ✅ 支持单个 DN 指定模版
            }
          ],
          "teamId": 1
        }
      ]
    }
  ]
}
```

### 当前实现的问题

**Go 实现** (`buildDNInstallList` 函数，第543-570行)：
- 没有为 OS 角色的 DN 节点添加 `templateName` 字段
- 应该使用 `row.Templates.DnOSTemplate`（`template_{server_type}_dn_OS.json`）

**Python 实现** (`build_dn_install_list` 函数，第200-222行)：
- 同样没有为 OS 角色添加 `templateName` 字段
- 这是历史遗留问题

### 正确的逻辑

根据模版设计：
- **普通角色（M, S, TS, LS）的 DN**：使用统一的 DN 模版（通过 `parameterTemplateInfos` 批量配置）
- **OS 角色的 DN**：应该在 `dnList` 中单独指定 `templateName` 为 `template_{server_type}_dn_OS.json`

### 需要修改的代码

```go
func buildDNInstallList(row normalizedRow, args runArgs) []map[string]any {
    installUser := fmt.Sprintf("%sdb1", args.Prefix)
    installPath := fmt.Sprintf("%s/%s", args.BasePath, installUser)
    dataPath := installPath + "/data"

    teamList := []map[string]any{}
    for _, role := range roleSequence {
        ip := row.RoleIPs[role]
        if ip == "" {
            continue
        }
        
        dnItem := map[string]any{
            "ip":          ip,
            "dbRole":      roleToDBRole[role],
            "installPath": installPath,
            "installUser": installUser,
            "dataPath":    dataPath,
        }
        
        // OS 角色需要指定专用 DN 模版
        if role == "OS" {
            dnItem["templateName"] = row.Templates.DnOSTemplate
        }
        
        teamList = append(teamList, map[string]any{
            "teamId": roleToTeamID[role],
            "dnList": []map[string]any{dnItem},
        })
    }
    return []map[string]any{{
        "dbgroupId": 1,
        "teamList":  teamList,
    }}
}
```

### 模版应用规则总结

根据接口文档的说明：

1. **批量配置**（`parameterTemplateInfos`）：
   - DN 类型模版应用于所有 DN 节点
   - CN 类型模版应用于所有 CN 节点
   
2. **单个组件配置**（`templateName` 字段）：
   - 单个组件的 `templateName` 优先级更高，会覆盖批量配置
   - OS 角色的 DN 应该通过此字段指定专用模版

### 实际应用场景

当创建包含 OS 角色的集群时：
- `parameterTemplateInfos` 中的 DN 模版 = `template_{server_type}_dn.json`（应用于 M, S, TS, LS）
- OS 角色的 DN 在 `dnList` 中单独指定 `templateName` = `template_{server_type}_dn_OS.json`（覆盖批量配置）

## 实施建议

### 已完成
1. ✅ **CN 端口逻辑修复**：M/S/TS 使用 3306+3307，LS 使用 3308，OS 使用 3309
2. ✅ **测试用例编写**：完整覆盖各角色端口分配
3. ✅ **编译验证**：代码编译通过

### 待实施
1. **DN 模版传入修复**：在 `buildDNInstallList` 中为 OS 角色的 DN 添加 `templateName` 字段
2. **测试用例**：验证 OS 角色的 DN 正确传入专用模版
3. **集成测试**：使用真实 CSV 数据验证完整请求体格式

## 相关文件

- Go 实现：`internal/insightbatchcreate/run.go`
- Python 参考：`scripts/batch_create_from_csv.py`
- 设计文档：`docs/insight_batch_create_design.md`
- 使用文档：`docs/insight_batch_create.md`
