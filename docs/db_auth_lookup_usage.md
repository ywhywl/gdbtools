# db-auth-lookup 使用说明

## 目标

`db-auth-lookup` 用于读取 4 份 Excel 台账，按业务名称查询该业务下全部数据库授权信息。

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

## 参数

- `--business-name`
  - 必填，业务名称
- `--business-cluster-file`
  - 必填，数据库集群映射表路径
- `--db-cluster-file`
  - 必填，数据库和集群映射表路径
- `--access-relation-file`
  - 必填，访问关系表路径
- `--app-ip-file`
  - 必填，应用和ip映射表路径
- `--output-format`
  - 可选，`text` 或 `json`
- `--with-diagnostics`
  - 可选，输出未匹配和解析告警

## 输出

明细输出字段包括：

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
- 状态

## 当前实现说明

当前版本已支持：

- 从 `数据库集群映射表` 按业务名称筛选集群
- 从 `数据库和集群映射表` 找到数据库名称
- 将 `访问关系表` 中的连续库表达式展开后匹配
- 从 `应用和ip映射表` 关联 IP
- 输出文本和 JSON 结果

当前版本对样例数据增加了一个兜底规则：

- 若 `数据库集群映射表.集群名` 未出现在 `数据库和集群映射表`
- 则尝试将如 `BJ13_clearing_branch_00` 这样的集群名裁剪为 `clearing_branch_00`
- 再去匹配 `访问关系表`

这个兜底是为了兼容当前样例截图中的字段关系，正式接真实台账时仍建议优先依赖完整的 `数据库和集群映射表`。
