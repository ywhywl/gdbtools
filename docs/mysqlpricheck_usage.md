# mysqlpricheck 使用手册

## 简介

`mysqlpricheck` 用于审计 MySQL 5.7 实例内部的用户权限，重点关注以下问题：

- 同一个用户在不同 host 下权限不一致
- 同一个用户在任意 host 下合并后同时拥有多个 schema 的权限
- 存在库级权限
- 存在表级权限

## 构建

```bash
go build ./cmd/mysqlpricheck
```

## 基本用法

```bash
go run ./cmd/mysqlpricheck \
  --target '10.0.0.11:3306' \
  --defaults-file /etc/my.cnf \
  --output-format text
```

## 多实例

```bash
go run ./cmd/mysqlpricheck \
  --target '10.0.0.11:3306|10.0.0.12:3306' \
  --user root \
  --password 'secret' \
  --output-format json
```

## 只检查指定用户

```bash
go run ./cmd/mysqlpricheck \
  --target '10.0.0.11:3306' \
  --users 'app_user,report_user@%' \
  --output-format text
```

账号检查口径为 `user`。例如 `app@%` 和 `app@10.0.0.%` 会作为同一个 `app` 账号统计和输出；`user@host` 选择器仅用于限定采集哪些 host 明细。

## 只检查 host 权限不一致

```bash
go run ./cmd/mysqlpricheck \
  --target '10.0.0.11:3306' \
  --check host_consistency
```

## 参数说明

- `--target`
  - 目标 MySQL 地址
  - 支持重复传入，支持 `,` `|` 和换行分隔
- `--config`
  - JSON 配置文件
- `--defaults-file`
  - MySQL 配置文件，例如 `/etc/my.cnf`
- `--user`
  - 用户名
- `--password`
  - 密码
- `--port`
  - 默认端口
- `--socket`
  - Unix socket
- `--connect-timeout`
  - 连接超时秒数
- `--users`
  - 仅检查指定用户，或用 `user@host` 限定指定 host 明细
- `--exclude-users`
  - 排除用户，或用 `user@host` 排除指定 host 明细
- `--exclude-schemas`
  - 排除 schema
- `--include-anonymous`
  - 包含匿名用户
- `--check`
  - `all|host_consistency|multi_schema|db_level|table_level`
- `--output-format`
  - `text|json`
- `--output`
  - 写入文件
- `--detail-limit`
  - 单实例输出的最大 finding 数
- `--fail-on`
  - `high|medium|low|none`

## 配置文件

### JSON 配置

```json
{
  "default_user": "root",
  "default_password": "secret",
  "default_port": 3306,
  "exclude_schemas": ["mysql", "information_schema", "performance_schema", "sys"],
  "exclude_users": ["mysql.sys", "mysql.session"]
}
```

### my.cnf

支持读取 `[client]` 段：

```ini
[client]
user=root
password=secret
port=3306
socket=/tmp/mysql.sock
```

## 输出示例

```text
Instance: root@10.0.0.11:3306
  Status: success
  Summary:
    checked_users=12
    checked_identities=18
    inconsistent_host_privilege_users=2
    multi_schema_users=5
    db_level_privilege_users=8
    table_level_privilege_users=3
  Findings:
    [HIGH] inconsistent_host_privileges
      user=app
      summary=app has different privileges across hosts
      detail={"hosts":["%","10.0.0.%"],"snapshots":[...]}
```

JSON 输出中暂时保留 `multi_schema_identities`、`db_level_privilege_identities`、`table_level_privilege_identities` 兼容旧消费方，但这些字段已经使用 user 口径，后续应迁移到对应的 `*_users` 字段。

## 权限要求

执行账号至少需要能查询：

- `mysql.user`
- `mysql.db`
- `mysql.tables_priv`

## 退出码

- `0`：未触发阈值
- `1`：命中中低风险
- `2`：命中高风险
- `3`：执行失败
