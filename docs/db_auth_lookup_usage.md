# db-auth-lookup 使用说明

## 目标

`db-auth-lookup` 用于读取 4 份台账，按业务名称查询该业务下全部数据库授权信息。输入文件支持 `.xlsx`、`.xlsm` 和 `.csv`，按文件后缀自动识别。

## 构建

```bash
go build ./cmd/db-auth-lookup
```

## 运行

基于仓库中的样例 Excel：

```bash
go run ./cmd/db-auth-lookup \
  --business-name gdb-trans \
  --business-cluster-file sample_excels/数据库集群映射表.xlsx \
  --db-cluster-file sample_excels/数据库和集群映射表.xlsx \
  --access-relation-file sample_excels/访问关系表.xlsx \
  --app-ip-file sample_excels/应用和ip映射表.xlsx \
  --with-diagnostics
```

输出格式切换为 JSON：

```bash
go run ./cmd/db-auth-lookup \
  --business-name gdb-trans \
  --business-cluster-file sample_excels/数据库集群映射表.xlsx \
  --db-cluster-file sample_excels/数据库和集群映射表.xlsx \
  --access-relation-file sample_excels/访问关系表.xlsx \
  --app-ip-file sample_excels/应用和ip映射表.xlsx \
  --output-format json \
  --with-diagnostics
```

输出格式切换为 CSV：

```bash
go run ./cmd/db-auth-lookup \
  --business-cluster-file sample_excels/数据库集群映射表.xlsx \
  --db-cluster-file sample_excels/数据库和集群映射表.xlsx \
  --access-relation-file sample_excels/访问关系表.xlsx \
  --app-ip-file sample_excels/应用和ip映射表.xlsx \
  --output-format csv \
  --with-diagnostics \
  --output /tmp/db_auth_lookup.csv
```

使用 CSV 作为输入：

```bash
go run ./cmd/db-auth-lookup \
  --business-cluster-file sample_excels/数据库集群映射表.csv \
  --db-cluster-file sample_excels/数据库和集群映射表.csv \
  --access-relation-file sample_excels/访问关系表.csv \
  --app-ip-file sample_excels/应用和ip映射表.csv \
  --output-format xlsx \
  --with-diagnostics \
  --output /tmp/db_auth_lookup.xlsx
```

输出格式切换为 Excel：

```bash
go run ./cmd/db-auth-lookup \
  --business-cluster-file sample_excels/数据库集群映射表.xlsx \
  --db-cluster-file sample_excels/数据库和集群映射表.xlsx \
  --access-relation-file sample_excels/访问关系表.xlsx \
  --app-ip-file sample_excels/应用和ip映射表.xlsx \
  --output-format xlsx \
  --with-diagnostics \
  --output /tmp/db_auth_lookup.xlsx
```

按数据库和应用维度聚合：

```bash
go run ./cmd/db-auth-lookup \
  --business-name gdb-trans \
  --business-cluster-file sample_excels/数据库集群映射表.xlsx \
  --db-cluster-file sample_excels/数据库和集群映射表.xlsx \
  --access-relation-file sample_excels/访问关系表.xlsx \
  --app-ip-file sample_excels/应用和ip映射表.xlsx \
  --aggregate-by database \
  --output-format xlsx \
  --output /tmp/db_auth_lookup_by_database.xlsx
```

按集群维度聚合：

```bash
go run ./cmd/db-auth-lookup \
  --business-name gdb-trans \
  --business-cluster-file sample_excels/数据库集群映射表.xlsx \
  --db-cluster-file sample_excels/数据库和集群映射表.xlsx \
  --access-relation-file sample_excels/访问关系表.xlsx \
  --app-ip-file sample_excels/应用和ip映射表.xlsx \
  --aggregate-by cluster \
  --output-format xlsx \
  --output /tmp/db_auth_lookup_by_cluster.xlsx
```

## 参数

- `--business-name`
  - 可选，业务名称
  - 不传时匹配全部业务
  - 可重复传入，也支持逗号分隔，例如 `--business-name gdb-trans,gdb-settle`
- `--business-cluster-file`
  - 必填，数据库集群映射表路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
  - 读取 `manager` 列，并作为结果第一列输出
- `--db-cluster-file`
  - 必填，数据库和集群映射表路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--access-relation-file`
  - 必填，访问关系表路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--app-ip-file`
  - 必填，应用和ip映射表路径
  - 支持 `.xlsx`、`.xlsm`、`.csv`
- `--output-format`
  - 可选，`text`、`json`、`csv` 或 `xlsx`
- `--output`
  - 可选，输出文件路径
  - 不传时输出到标准输出
  - `--output-format csv` 或 `--output-format xlsx` 时必填
- `--aggregate-by`
  - 可选，`detail`、`database` 或 `cluster`
  - 默认 `detail`，不聚合，保持授权明细输出
  - `database` 按 `manager + 业务名称 + 数据库类型 + 集群名 + 主库 + 数据库名称 + 应用名称-CMDB` 聚合
  - `cluster` 按 `manager + 业务名称 + 数据库类型 + 集群名 + 主库` 聚合
- `--with-diagnostics`
  - 可选，输出未匹配和解析告警
  - 使用 `--output` 写文件时，诊断信息仍输出到标准输出，不写入结果文件

## 输出

命令标准输出默认打印统计信息。即使使用 `--output` 将明细写入文件，统计信息仍输出到终端。

标准输出示例：

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

使用 `--with-diagnostics` 时，会在统计信息后追加诊断信息：

```text
Diagnostics:
- missing_cluster_mapping [BJ13_clearing_branch_27]: cluster not found in 数据库和集群映射表: BJ13_clearing_branch_27
- missing_cluster_mapping [BJ13_clearing_branch_28]: cluster not found in 数据库和集群映射表: BJ13_clearing_branch_28
- missing_cluster_mapping [BJ13_clearing_branch_29]: cluster not found in 数据库和集群映射表: BJ13_clearing_branch_29
```

明细输出字段包括：

- manager
- 业务名称
- 数据库类型
- 集群名
- 主库
- 数据库名称
- 应用名称-CMDB
- 应用所属中心
- IP
- 访问数据库使用用户
- 访问权限
- 备注
- 状态

聚合输出规则：

- `--aggregate-by detail`：一条授权关系一行，不做聚合
- `--aggregate-by database`：按 manager、数据库和应用聚合，合并应用所属中心、数据库主库所属中心、目标节点数据库角色、IP、访问用户、访问权限、备注、状态和告警
- `--aggregate-by cluster`：按 manager 和集群聚合，额外合并数据库名称和应用名称
- `manager` 是聚合分组 key，不同 manager 的记录不会合并到同一行，便于按 manager 分别执行添加白名单操作
- 被合并字段会去空值、去重并稳定排序
- Excel 输出中 IP 使用单元格内换行展示，多值名称、IDC、角色、用户、备注、状态和告警在聚合输出中也使用单元格内换行展示

IDC 归一化规则：

- `应用所属中心`、`数据库主库所属中心`、`应用和ip映射表.应用所属中心` 都会归一化后参与匹配
- `13` 保持为 `13`
- `BJ13`、`bj13` 归一化为 `13`
- `SH02`、`sh02` 归一化为 `02`
- 无法识别成两位 IDC 的值会保留清洗后的原值

统计口径：

- 业务数量：最终命中的业务名称去重数量
- 集群数量：业务过滤后命中的集群名去重数量
- 数据库数量：成功匹配出的数据库名称去重数量
- 授权明细数量：最终输出的授权明细行数
- 应用数量：最终授权明细中的 `应用名称-CMDB` 去重数量
- 应用服务器 IP 数量：最终授权明细关联到的 IP 去重数量
- By business：按 `业务名称 + 应用所属中心` 拆行，统计每个业务在每个 IDC 下的应用数量、IP 数量和授权明细数量

## 当前实现说明

当前版本已支持：

- 从 `数据库集群映射表` 按业务名称筛选集群
- 从 `数据库集群映射表` 读取 manager 并作为聚合分组 key
- 不传 `--business-name` 时查询全部业务
- 多次传入 `--business-name` 或使用逗号分隔时查询多个业务
- 从 `数据库和集群映射表` 找到数据库名称
- 将 `访问关系表` 中的连续库表达式展开后匹配
- 将 IDC 代号归一化后匹配应用 IP
- 从 `应用和ip映射表` 关联 IP
- 支持按明细、数据库和集群三种维度输出
- 输出文本、JSON、CSV 和 Excel 结果
- 输入支持 Excel 和 CSV

当前版本严格依赖 `数据库和集群映射表` 中的 `集群名 -> 数据库名称` 映射关系。若 `数据库集群映射表` 中的集群名无法在该表中找到，会记录为 `missing_cluster_mapping`。
