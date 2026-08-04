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

## 实施建议

1. **暂不实现**端口逻辑修改，等待明确需求
2. 优先确认以下信息：
   - LS/OS 的实际端口规则（从 Insight API 文档或实际部署中验证）
   - LS 是否需要专用 CN 模版
   - 用户名 suffix 的规则
3. 创建测试用例验证新逻辑
4. 更新文档说明端口分配规则

## 相关文件

- Go 实现：`internal/insightbatchcreate/run.go`
- Python 参考：`scripts/batch_create_from_csv.py`
- 设计文档：`docs/insight_batch_create_design.md`
- 使用文档：`docs/insight_batch_create.md`
