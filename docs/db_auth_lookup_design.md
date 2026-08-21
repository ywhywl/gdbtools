# db-auth-lookup 设计文档

## 目标

`db-auth-lookup` 是一个用于离线分析台账的授权查询工具。工具通过输入“数据库集群映射表”中的业务名称，串联 4 份表格中的映射关系，输出该业务下所有数据库授权信息。

目标输出字段：

- manager
- 业务名称
- 数据库类型
- 集群名
- 主库
- 数据库名称
- 应用和 IP 映射表中的 IP 信息
- 访问数据库使用的用户
- 访问权限

## 数据来源

第一版依赖以下 4 份表格，输入支持 `.xlsx`、`.xlsm` 和 `.csv`，按文件后缀自动识别：

- `数据库集群映射表`
- `数据库和集群映射表`
- `访问关系表`
- `应用和ip映射表`

用户提供的是 Excel 或 CSV 文件，当前需求描述中附带的是对应截图。后续实现时应直接读取原始表格文件，而不是依赖图片。

根据当前 4 张截图，已确认的关键列名如下。

### 1. 应用和ip映射表

截图中可见列：

- `应用名称-CMDB`
- `应用所属中心`
- `IP`

### 2. 数据库集群映射表

截图中可见列：

- `部门`
- `manager`
- `业务名称`
- `数据库类型`
- `集群名`
- `主库`
- `备库`
- `临时备`
- `同城备`
- `异地备`

### 3. 数据库和集群映射表

截图中可见列：

- `集群名`
- `数据库名称`
- `数据库类型`

### 4. 访问关系表

截图中可见列：

- `序号`
- `应用名称-CMDB`
- `应用所属中心`
- `数据库名称`
- `数据库主库所属中心`
- `目标节点数据库角色`
- `访问数据库使用用户`
- `访问权限`

## 业务问题理解

4 份表分别承担不同职责：

- `数据库集群映射表`
  - 提供“业务名称 -> 数据库名称”的起点关系
- `数据库和集群映射表`
  - 提供“数据库名称 -> 集群信息（集群名、主库、数据库类型等）”的映射
- `访问关系表`
  - 提供“数据库名称 -> 应用 -> 访问用户 -> 访问权限”的授权关系
- `应用和ip映射表`
  - 提供“应用 -> IP”的补充信息

最终查询本质上是一次多表离线关联：

1. 先从 `数据库集群映射表` 中按业务名称找到目标数据库集合
2. 再用数据库名称匹配 `数据库和集群映射表`
3. 再用归一化后的数据库名称匹配 `访问关系表`
4. 最后用应用名匹配 `应用和ip映射表`，补齐 IP 信息

## 范围

第一版包含：

- 按业务名称查询
- 支持 Excel 和 CSV 输入
- Excel 读取第一个 sheet，CSV 第一行作为表头
- 支持数据库名称规范化后匹配
- 输出业务下全部命中的授权明细
- 同时保留原始数据库名和归一化数据库名，便于排查

第一版不包含：

- 反向按 IP、应用名、用户名查询
- 自动修复脏数据
- 数据库实时连库校验
- 权限语义转换为更细粒度的 SQL 权限集合
- 从图片 OCR 提取表格

## 核心难点

核心难点不在 Excel 读取，而在不同表中数据库名称表达方式不一致，尤其是连续分库名称需要归一化后再匹配。

已知重点规则：

- `clearing_branch_00至clearing_branch_29`
- `clearing_branch_00-clearing_branch_29`
- `clearing_branch_00-29`

上述写法语义等价，均表示连续的 30 套库：

- `clearing_branch_00`
- `clearing_branch_01`
- ...
- `clearing_branch_29`

因此，访问关系表中的数据库名称字段不能直接按原值等值匹配，必须先转换为标准数据库名集合，再展开为“一行一个数据库”的明细记录参与关联。

## 数据模型理解

建议在实现中统一抽象出以下中间模型。

### 1. 业务数据库映射

来自 `数据库集群映射表`：

- `manager`
- `business_name`
- `database_name_raw`
- `database_name_normalized[]`

说明：

- 一条原始记录可能对应一个数据库，也可能对应一组展开后的数据库

### 2. 数据库集群信息

来自 `数据库和集群映射表`：

- `database_name_raw`
- `database_name_normalized`
- `database_type`
- `cluster_name`
- `primary_host`

说明：

- 该表是数据库基础属性表
- 后续输出中的数据库类型、集群名、主库主要从这里取值

### 3. 数据库访问关系

来自 `访问关系表`：

- `database_name_raw`
- `database_name_normalized`
- `application_name`
- `db_user`
- `privilege`

说明：

- 原始数据库名可能是单库，也可能是一个范围表达式
- 入库前应先展开成一行一个标准数据库名

### 4. 应用 IP 映射

来自 `应用和ip映射表`：

- `application_name`
- `ip_list[]`

说明：

- 一个应用可能对应多个 IP
- 输出时可保留为逗号拼接字符串，也可保留数组，取决于最终输出格式

## 名称归一化规则

数据库名称归一化是本方案的关键。

### 基本原则

- 匹配时使用 `normalized_name`
- 输出时同时保留原始值 `raw_name`
- 所有参与关联的表都要走同一套归一化逻辑

### 规则分类

#### 1. 单库名称

示例：

- `clearing_branch_00`

处理：

- 直接识别为单个数据库名

#### 2. 显式区间

示例：

- `clearing_branch_00至clearing_branch_29`
- `clearing_branch_00-clearing_branch_29`

处理：

- 识别前缀 `clearing_branch_`
- 识别起始编号 `00`
- 识别结束编号 `29`
- 按编号宽度补零展开

输出展开结果：

- `clearing_branch_00`
- `clearing_branch_01`
- ...
- `clearing_branch_29`

#### 3. 简写区间

示例：

- `clearing_branch_00-29`

处理：

- 左侧提供完整起始库名
- 右侧仅提供结束编号
- 结束编号应继承起始编号的宽度
- 根据前缀和编号位数展开为完整数据库名列表

### 归一化约束

- 起止前缀必须一致，否则记为非法范围
- 起始编号必须小于等于结束编号，否则记为非法范围
- 编号位数不一致时，优先以起始编号位数为标准
- 若存在中文空格、英文空格、全角连接符，需要先做清洗

## 处理流程

### 1. 输入加载

读取 4 个表格文件，提取目标列并映射到内部结构。

基于当前截图，第一版可以直接按真实列名读取：

- `数据库集群映射表`
  - `业务名称`
  - `数据库类型`
  - `集群名`
  - `主库`
- `数据库和集群映射表`
  - `集群名`
  - `数据库名称`
  - `数据库类型`
- `访问关系表`
  - `应用名称-CMDB`
  - `应用所属中心`
  - `数据库名称`
  - `数据库主库所属中心`
  - `目标节点数据库角色`
  - `访问数据库使用用户`
  - `访问权限`
- `应用和ip映射表`
  - `应用名称-CMDB`
  - `应用所属中心`
  - `IP`

实现上仍建议保留一个小型列名别名字典，以防实际 Excel 标题和截图存在细微差异。

### 2. 原始数据清洗

清洗内容包括：

- 去除首尾空白
- 统一中英文连接符
- 统一 `至`、`-` 等范围表达
- 去除无意义空值行

### 3. 数据库名称展开

对以下来源的数据库名称字段执行统一展开：

- `数据库和集群映射表`
- `访问关系表`

其中最关键的是 `访问关系表`，需要将范围表达式拆成一行一个数据库名。

`数据库集群映射表` 当前截图中没有直接出现 `数据库名称` 字段，因此第一版主链路应调整为：

1. 按输入业务名称从 `数据库集群映射表` 取出该业务对应的 `集群名`
2. 再通过 `数据库和集群映射表` 将 `集群名` 转成一个或多个 `数据库名称`
3. 再用归一化后的 `数据库名称` 去匹配 `访问关系表`
4. 最后用 `应用名称-CMDB` 去匹配 `应用和ip映射表`

### 4. 业务过滤

按用户输入的业务名称，从 `数据库集群映射表` 中取出该业务关联的全部集群信息，至少包括：

- `manager`
- `业务名称`
- `数据库类型`
- `集群名`
- `主库`

### 5. 集群信息匹配

用上一步得到的 `集群名` 匹配 `数据库和集群映射表`，展开出该业务对应的全部标准数据库名，并补齐：

- 集群名
- 数据库名称
- 数据库类型

其中：

- `数据库类型` 优先可取 `数据库集群映射表`
- 若两张表都存在该字段，可做一致性校验
- `主库` 取自 `数据库集群映射表`

### 6. 授权信息匹配

继续用标准数据库名匹配 `访问关系表`，获取：

- 应用名称-CMDB
- 应用所属中心
- 数据库主库所属中心
- 目标节点数据库角色
- 访问数据库用户
- 访问权限

### 7. IP 信息匹配

用访问关系中的 `应用名称-CMDB` 匹配 `应用和ip映射表`，补齐 IP 信息。

如同一个应用在不同 `应用所属中心` 下可能存在不同 IP，建议匹配键优先使用复合键：

- `应用名称-CMDB`
- `应用所属中心`

### 8. 结果汇总输出

最终按一条授权关系输出一行结果，建议字段为：

- `business_name`
- `database_type`
- `cluster_name`
- `primary_host`
- `database_name`
- `application_name`
- `application_center`
- `database_primary_center`
- `database_role`
- `ip`
- `db_user`
- `privilege`

## 匹配规则

建议按以下优先顺序做关联。

### 1. 业务到数据库

关联键：

- `数据库集群映射表.集群名`

过滤条件：

- `数据库集群映射表.business_name == 输入业务名称`

### 2. 数据库到集群

关联键：

- `集群名`

规则：

- 用 `数据库集群映射表.集群名`
- 匹配 `数据库和集群映射表.集群名`
- 取出一对多的 `数据库名称`

### 3. 数据库到访问关系

关联键：

- `数据库名称`

规则：

- 访问关系表需先展开连续库范围
- 展开后再按标准数据库名等值匹配

### 4. 应用到 IP

关联键：

- `应用名称-CMDB`
- 归一化后的 `应用所属中心`

规则：

- 建议先做应用名去空白标准化
- `应用所属中心` 先做 IDC 归一化，`BJ13`、`bj13`、`13` 统一为 `13`
- 如应用名唯一，可退化为只按 `应用名称-CMDB` 匹配

### IDC 归一化

以下字段代表 IDC 代号，参与匹配前统一归一化：

- `访问关系表.应用所属中心`
- `访问关系表.数据库主库所属中心`
- `应用和ip映射表.应用所属中心`

归一化规则：

- `13` -> `13`
- `BJ13` -> `13`
- `bj13` -> `13`
- `SH02` -> `02`
- `sh02` -> `02`
- 空值保持空值
- 无法识别成两位 IDC 的值保留清洗后的原值

## 输出口径

建议输出拆成两层：

- 标准输出：始终输出本次查询的统计摘要
- 结果文件：按 `--output-format` 输出授权明细，例如 `text`、`json`、`csv`、`xlsx`

这样在使用 `--output-format xlsx --output xx.xlsx` 或 `--output-format csv --output xx.csv` 时，结果文件保持为可交付的授权明细，终端仍能直接看到本次查询规模、应用服务器 IP 分布和诊断信息。

### 明细视图

一条授权关系一行，适合排查和导出。

示例字段：

- manager
- 业务名称
- 数据库类型
- 集群名
- 主库
- 数据库名称
- 应用名
- 应用所属中心
- 数据库主库所属中心
- 目标节点数据库角色
- IP
- 访问用户
- 访问权限
- 备注

### 聚合视图

通过 `--aggregate-by` 控制聚合口径：

- `detail`：默认值，不聚合，保持明细视图
- `database`：按 `manager + 业务名称 + 数据库类型 + 集群名 + 主库 + 数据库名称 + 应用名称-CMDB` 聚合
- `cluster`：按 `manager + 业务名称 + 数据库类型 + 集群名 + 主库` 聚合

`manager` 是聚合分组 key，不作为普通合并字段。不同 `manager` 的授权结果必须拆成不同行，便于最终按每个单独的 manager 执行添加白名单操作。

`database` 聚合时合并以下字段：

- 应用所属中心
- 数据库主库所属中心
- 目标节点数据库角色
- IP
- 访问用户
- 访问权限
- 备注
- 状态
- 告警

`cluster` 聚合时还会额外合并：

- 数据库名称
- 应用名称-CMDB

合并规则：

- 去空值
- 去重
- 稳定排序
- CSV/text 中多值以逗号展示
- XLSX 中 IP 使用单元格内换行展示，其他聚合多值字段优先使用单元格内换行展示

### 标准输出统计视图

统计视图默认输出，不依赖 `--with-diagnostics`。

总体统计建议包含：

- 业务数量
- 集群数量
- 数据库数量
- 授权明细数量
- 应用数量
- 应用服务器 IP 数量

按业务统计建议包含，并按 `应用所属中心` 拆行：

- 业务名称
- 集群数量
- 数据库数量
- 应用所属中心，显示为 `idc-<应用所属中心>`
- 授权明细数量
- 应用数量
- 应用服务器 IP 数量

示例：

```text
Summary:
Businesses: 1
Clusters: 30
Databases: 2
Authorization rows: 6
Applications: 1
Application IPs: 3

By business:
- gdb-trans: clusters=30 databases=2 idc-13 applications=1 ips=3 authorization_rows=2
- gdb-trans: clusters=30 databases=2 idc-12 applications=1 ips=3 authorization_rows=2
- gdb-trans: clusters=30 databases=2 idc-23 applications=1 ips=3 authorization_rows=2
```

统计口径：

- `Businesses`
  - 最终命中的业务名称去重数量
- `Clusters`
  - `数据库集群映射表` 中命中的集群名去重数量
- `Databases`
  - 通过 `数据库和集群映射表` 成功匹配出的数据库名称去重数量
- `Authorization rows`
  - 最终输出的授权明细行数
- `Applications`
  - 最终授权明细中的 `应用名称-CMDB` 去重数量
- `Application IPs`
  - 最终授权明细关联到的 IP 去重数量
- `By business`
  - 先按业务名称分组，再按 `访问关系表.应用所属中心` 拆分为多行
  - 每行显示该业务在该 `应用所属中心` 下的应用数量、IP 数量、授权明细行数
  - `clusters` 和 `databases` 表示该业务整体涉及的集群数量和数据库数量，不按 IDC 缩减

### 诊断视图

诊断视图只在传入 `--with-diagnostics` 时输出，并追加在标准输出统计之后。

诊断信息不写入 Excel 结果文件，避免和授权明细混在一起。

示例：

```text
Diagnostics:
- missing_cluster_mapping [BJ13_clearing_branch_27]: cluster not found in 数据库和集群映射表: BJ13_clearing_branch_27
- missing_cluster_mapping [BJ13_clearing_branch_28]: cluster not found in 数据库和集群映射表: BJ13_clearing_branch_28
- missing_cluster_mapping [BJ13_clearing_branch_29]: cluster not found in 数据库和集群映射表: BJ13_clearing_branch_29
```

### 汇总视图

按业务名称输出统计信息，适合快速审阅。

示例统计：

- 该业务涉及数据库数量
- 该业务涉及集群数量
- 该业务涉及应用数量
- 该业务授权记录数量
- 未匹配到集群信息的数据库数量
- 未匹配到 IP 的应用数量

## 异常与脏数据处理

第一版需要明确保留未匹配信息，不能静默丢弃。

建议分类：

- `missing_cluster_mapping`
  - 在 `数据库集群映射表` 中出现的 `集群名`，未匹配到 `数据库和集群映射表`
- `missing_access_relation`
  - 在业务数据库集合中出现的标准数据库名，未匹配到 `访问关系表`
- `missing_ip_mapping`
  - 在访问关系中出现的 `应用名称-CMDB(+应用所属中心)`，未匹配到 `应用和ip映射表`
- `invalid_database_range`
  - 数据库范围表达式无法解析

建议输出中增加可选诊断字段：

- `match_status`
- `warning`

## 实现建议

如果后续进入编码阶段，建议作为新的 Go CLI 实现，保持与仓库现有结构一致。

推荐目录：

```text
cmd/db-auth-lookup/
  main.go

internal/dbauthlookup/
  run.go
  types.go
  excel.go
  normalize.go
  matcher.go
  report.go
  errors.go

docs/
  db_auth_lookup_design.md
  db_auth_lookup_usage.md
```

模块职责建议：

- `run.go`
  - 参数解析、执行流程、退出码
- `excel.go`
  - 按文件后缀分派 Excel/CSV 读取，并处理 4 份表的列映射
- `normalize.go`
  - 数据库名称清洗、区间解析、展开
- `matcher.go`
  - 业务过滤和多表关联
- `report.go`
  - 标准输出统计、诊断信息、文本、CSV、JSON 输出
- `xlsx.go`
  - Excel 明细文件输出
- `types.go`
  - 中间模型和输出模型

### 统计实现建议

建议在多表关联完成后统一计算统计，不在 Excel 读取阶段累计。

推荐新增统计模型：

```text
ConsoleSummary
  Total
  ByBusiness[]
  ByBusinessIDC[]

ConsoleTotal
  business_count
  cluster_count
  database_count
  authorization_count
  application_count
  ip_count

BusinessIDCSummary
  business_name
  application_center
  cluster_count
  database_count
  authorization_count
  application_count
  ip_count
```

统计来源：

- 总体和按业务统计基于最终 `ResultRow`
- 集群数量也应参考业务过滤后的 `BusinessClusterRow`
- `By business` 统计需要按 `ResultRow.BusinessName + ResultRow.ApplicationCenter` 分组
- IP 数量基于 `ResultRow.IPs` 去重

输出规则：

- 使用 `--output` 写结果文件时，标准输出打印统计信息
- `csv` 和 `xlsx` 输出必须指定 `--output`，避免标准输出同时混合机器可读明细和统计文本
- `--with-diagnostics` 时，在统计信息后追加诊断信息
- `xlsx` 文件只写授权明细，不写统计和诊断
- `csv` 文件只写授权明细，不写统计和诊断
- `json` 可继续输出完整结构化结果，但标准输出统计仍应保持可读文本

## 建议参数设计

如果后续做 CLI，建议支持：

- `--business-name`
  - 可选，目标业务名称
  - 不传时匹配全部业务
  - 支持重复传入或逗号分隔以查询多个业务
- `--business-cluster-file`
  - `数据库集群映射表` 路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--db-cluster-file`
  - `数据库和集群映射表` 路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--access-relation-file`
  - `访问关系表` 路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--app-ip-file`
  - `应用和ip映射表` 路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--output-format`
  - `text` / `json` / `csv` / `xlsx`
- `--output`
  - 输出文件路径
  - `csv` 和 `xlsx` 格式必填
- `--aggregate-by`
  - `detail` / `database` / `cluster`
  - 默认 `detail`
- `--with-diagnostics`
  - 是否输出未匹配与告警信息

## 测试重点

后续实现时，测试重点应放在名称归一化和多表匹配，而不是 Excel I/O 本身。

至少覆盖：

- 单库名称解析
- `A_00至A_29` 展开
- `A_00-A_29` 展开
- `A_00-29` 展开
- 非法范围识别
- 一个业务对应多个数据库
- 一个应用对应多个 IP
- IDC 代号 `BJ13`、`bj13`、`13` 归一化后一致匹配
- `--aggregate-by detail` 保持明细输出
- `--aggregate-by database` 按 manager、数据库和应用维度聚合
- `--aggregate-by cluster` 按 manager 和集群维度聚合
- manager 作为聚合分组 key，不同 manager 不合并
- 聚合字段去空、去重并稳定排序
- 数据库命中集群但未命中访问关系
- 访问关系命中但应用未命中 IP 映射
- CSV 输入与 Excel 输入解析结果一致
- CSV 表头带 UTF-8 BOM 时仍能匹配真实列名
- 非法 `--aggregate-by` 返回参数错误

## 待确认项

当前需求已经足够完成方案设计，但编码前还需要确认以下细节：

- `应用和ip映射表` 中一个应用对应多个 IP 时，原表是单元格多值还是多行
- 访问权限字段是否已经是最终展示值，还是需要从多个权限列拼装
- 业务名称是否允许模糊匹配，还是只做精确匹配
- 同一个 `应用名称-CMDB` 在不同 `应用所属中心` 下是否可能重复，若重复则必须使用复合键匹配 IP
- `数据库集群映射表` 中同一业务是否可能对应多个集群名

## 结论

该需求本质是一个基于 Excel 台账的离线授权检索工具，关键不在于读取表格，而在于统一数据库名称表达并完成稳定的多表关联。

实现上应先建立统一的数据库名称归一化与区间展开能力，再围绕“业务名称 -> 数据库 -> 集群 -> 访问关系 -> 应用 IP”的链路组织查询流程。只要这个归一化能力设计正确，后续 CLI、导出、统计和异常诊断都可以在同一套中间模型上稳定扩展。
