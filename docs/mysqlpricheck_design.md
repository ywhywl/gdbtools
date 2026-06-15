# mysqlpricheck 设计文档

## 目标

`mysqlpricheck` 是一个面向 MySQL 5.7 的单实例权限审计工具，用于发现实例内部用户授权不一致和高维护成本的权限配置。工具使用 Go 实现，支持一次审计多个实例，但每个实例独立执行、独立输出结果。

## 范围

第一版采集并检查以下授权来源：

- `mysql.user`：全局权限
- `mysql.db`：库级权限
- `mysql.tables_priv`：表级权限

第一版不覆盖：

- `mysql.columns_priv`
- `mysql.procs_priv`
- `mysql.proxies_priv`
- roles
- 动态权限

## 命令结构

- 入口：`cmd/mysqlpricheck/main.go`
- 核心实现：`internal/mysqlpricheck/`

模块划分：

- `run.go`：参数解析、执行流程、退出码
- `config.go`：JSON 配置和 `my.cnf` 解析
- `client.go`：MySQL 连接和查询封装
- `collector.go`：用户与权限采集
- `rules.go`：权限检查规则
- `report.go`：文本和 JSON 输出
- `types.go`：内部模型定义

## 凭据加载

支持以下来源，优先级从高到低：

1. CLI 参数：`--user` `--password` `--port` `--socket`
2. `--defaults-file`
3. 自动探测的 MySQL 配置文件
4. `--config` JSON 文件

自动探测路径：

- `/etc/my.cnf`
- `/etc/mysql/my.cnf`
- `~/.my.cnf`

`my.cnf` 只解析 `[client]` 段中的：

- `user`
- `password`
- `host`
- `port`
- `socket`

## 核心数据模型

审计账号单位是 `user`，同一个 `user` 下的多个 host 视为同一个账号。采集快照单位仍是 `user@host`，统一映射到 `PrivilegeSnapshot`：

- 全局权限：`GlobalPrivileges`
- 库级权限：`DBPrivileges`
- 表级权限：`TablePrivileges`

规则检查通过快照数组完成。除 host 权限一致性检查天然需要比较同一 user 下不同 host 的快照外，其他规则也按 `user` 聚合输出，host 仅作为定位明细保留。

## 检查规则

### 1. `inconsistent_host_privileges`

含义：

- 同一个 `user` 在多个 `host` 下的权限内容不一致
- `host` 字符串本身不参与差异比较，相同权限的 `user@host1` 与 `user@host2` 不报错

严重级别：

- `high`

检查内容：

- 全局权限
- 库级权限
- 表级权限

### 2. `multi_schema_privileges`

含义：

- 同一个 `user` 在任意 host 下合并后拥有多个 schema 的库级权限

严重级别：

- `medium`

### 3. `db_level_privileges`

含义：

- 按 `user` 合并所有 host 后，检查库级授权是否同时涉及多个不同 `user` 和多个不同 database
- 单个用户存在库级授权不输出该规则
- 多个用户只拥有同一个 database 的库级授权不输出该规则

严重级别：

- `medium`

### 4. `table_level_privileges`

含义：

- 列出所有存在表级直接授权的 `user`

严重级别：

- `medium`

## 过滤规则

默认排除系统库：

- `mysql`
- `information_schema`
- `performance_schema`
- `sys`

支持：

- `--users`
- `--exclude-users`
- `--exclude-schemas`
- `--include-anonymous`

## 输出格式

支持：

- `text`
- `json`

实例级报告包含：

- `instance`
- `summary`
- `findings`
- `error`

汇总字段包括：

- 检查用户数
- 检查采集身份数，即底层 `user@host` 快照数
- 各类规则命中数量
- 各严重级别数量

JSON 汇总字段中保留旧的 `*_identities` 字段用于兼容，但其值已经等同于新的 `*_users` user 口径字段。

## 退出码

- `0`：未触发阈值或 `--fail-on none`
- `1`：命中 `medium` 或 `low` 阈值
- `2`：命中 `high`
- `3`：执行失败，例如连接失败、查询失败

`--fail-on` 支持：

- `high`
- `medium`
- `low`
- `none`

## 实现说明

为兼容 MySQL 5.7 的不同小版本，工具动态读取 `mysql.user` 和 `mysql.db` 中的 `_priv` 列，而不是写死列名。表级权限来自 `mysql.tables_priv.Table_priv` 并在内存中归一化为大写权限集合。
