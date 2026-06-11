# mysqlcompare 使用说明

## 简介

`mysqlcompare` 用于比较源端和目标端 MySQL 的表结构与权限差异。

支持：

- 表结构比较
- 权限比较
- 多目标实例
- 文本和 JSON 输出

## 构建与运行

```bash
go build ./cmd/mysqlcompare
go run ./cmd/mysqlcompare --help
```

远程安装：

```bash
go install github.com/ywhywl/gdbtools/cmd/mysqlcompare@latest
```

## 连接输入

支持以下输入形式：

- 完整 DSN，例如 `mysql://root:password@10.0.0.11:3306/`
- 简化地址，例如 `10.0.0.11`，默认端口 `3306`
- 简化地址，例如 `10.0.0.11:3307`
- 多个目标通过 `,`、`|` 或换行分隔

凭据优先级：

1. DSN 中的用户名和密码
2. `--default-user` 和 `--default-password`
3. `--config` JSON 文件

配置文件示例：

```json
{
  "default_user": "root",
  "default_password": "password"
}
```

## 使用示例

```bash
go run ./cmd/mysqlcompare \
  --config ./mysqlcompare.json \
  --source-dsn '10.0.0.11' \
  --target-dsn $'10.0.0.12|10.0.0.13:3307' \
  --source-schemas 'dbname_0' \
  --target-schemas 'dbname_*' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys' \
  --users 'app_user,report_user@*' \
  --exclude-users 'mysql.session,mysql.sys' \
  --user-match-mode user \
  --check all \
  --output-format text
```

## 参数概览

- `--source-dsn`
  - 源端 MySQL，必填
- `--target-dsn`
  - 目标端 MySQL，必填，可重复
- `--config`
  - JSON 配置文件，支持 `default_user` 和 `default_password`
- `--default-user`
  - DSN 未提供账号时的默认用户名
- `--default-password`
  - DSN 未提供密码时的默认密码
- `--source-schemas` / `--source-databases`
  - 源端 schema 选择器
- `--target-schemas` / `--target-databases`
  - 目标端 schema 选择器
- `--exclude-schemas` / `--exclude-databases`
  - 排除 schema 选择器
- `--users`
  - 用户选择器，支持 `user` 和 `user@host`
- `--exclude-users`
  - 排除用户选择器
- `--user-match-mode`
  - `user` 或 `user_host`
- `--check`
  - `all`、`structure`、`privileges`
- `--output-format`
  - `text` 或 `json`

## 权限比较说明

1. 全局权限，例如 `GRANT SELECT ON *.*`，直接按同一用户比较，不依赖 schema 映射。
2. 库级权限，例如 `GRANT SELECT ON db_name.*`，只在已匹配的 schema 对之间比较。
3. 表级权限，例如 `GRANT SELECT ON db_name.orders`，只在已匹配的 schema 对之间比较。
4. 如果源端 `db0` 映射到目标端 `db1`，则源端在 `db0` 上的授权只会与目标端在 `db1` 上的授权比较。
5. 与 schema 对无关的其他目标端 schema 授权会被忽略，即使名称与源端 schema 相同。
6. 如果无法形成任何 schema 对，则只比较全局权限，跳过库级和表级权限。

映射示例：

- 源端选择器：`--source-schemas 'db0'`
- 目标端选择器：`--target-schemas 'db*'`
- 如果形成的 schema 对是 `db0 -> db1`，则源端 `user1` 在 `db0` 上的库级权限必须与目标端 `user1` 在 `db1` 上的库级权限一致
- 目标端 `user1` 在其他 schema，例如 `db0` 上的权限，不会用于满足 `db0 -> db1` 这组比较
