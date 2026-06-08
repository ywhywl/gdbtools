mysqlcompare Go 工具使用说明

1. 工具说明

mysqlcompare 是一个 Go 实现的 MySQL 对比工具，用于比较源端与一个或多个目标端的库结构和用户权限。

支持的检查类型：
- 结构对比
- 权限对比
- 结构 + 权限同时对比

支持的权限范围：
- 全局权限
- 库级权限
- 表级权限


2. 运行方式

编译：

go build ./cmd/mysqlcompare

直接运行：

go run ./cmd/mysqlcompare --help


3. 连接参数

支持的连接输入格式：

- 完整 DSN，例如：
  mysql://root:password@10.0.0.11:3306/

- 简化地址，例如：
  10.0.0.11

- 带端口的简化地址，例如：
  10.0.0.11:3307

说明：
- 简化地址默认端口为 3306
- 如果 DSN 中未提供用户名密码，可以通过默认参数或配置文件补充

凭据优先级：
1. DSN 中显式提供的用户名和密码
2. --default-user 和 --default-password
3. --config 指定 JSON 配置文件中的 default_user 和 default_password

配置文件示例：

{
  "default_user": "root",
  "default_password": "password"
}


4. 命令参数

基础参数：

- --source-dsn
  源库连接，必填

- --target-dsn
  目标库连接，必填，可重复传入

- --config
  JSON 配置文件，可提供 default_user 和 default_password

- --default-user
  默认用户名

- --default-password
  默认密码

库筛选参数：

- --source-schemas 或 --source-databases
  源端库选择器

- --target-schemas 或 --target-databases
  目标端库选择器

- --exclude-schemas 或 --exclude-databases
  排除的库选择器

用户筛选参数：

- --users
  只比较指定用户，支持 user 或 user@host
  可以不传
  不传时表示比较源端和目标端命中的全部用户

- --exclude-users
  排除指定用户
  可单独使用
  当 --users 不传时，表示从全部用户中排除指定用户

- --user-match-mode
  用户匹配模式：
  user_host
  user

检查控制参数：

- --check
  可选值：
  all
  structure
  privileges

- --output-format
  可选值：
  text
  json


5. 选择器匹配规则

库名、用户名等选择器使用 Linux shell glob 语义：
- *
- ?
- []

示例：
- db*
- db?
- db[0-9]

注意：
- 不是 MySQL LIKE 语义
- % 和 _ 不会被当作通配符
- 如果选择器正好命中一个实际存在的库名或用户名，会优先按精确匹配处理


6. 示例命令

示例 1：结构和权限一起比较

go run ./cmd/mysqlcompare \
  --config ./mysqlcompare.json \
  --source-dsn '10.0.0.11' \
  --target-dsn $'10.0.0.12|10.0.0.13:3307' \
  --source-schemas 'db0' \
  --target-schemas 'db*' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys' \
  --users 'app_user,report_user@*' \
  --exclude-users 'mysql.session,mysql.sys' \
  --user-match-mode user \
  --check all \
  --output-format text

示例 2：只比较权限

go run ./cmd/mysqlcompare \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn '10.0.0.11' \
  --target-dsn '10.0.0.12' \
  --source-schemas 'db0' \
  --target-schemas 'db*' \
  --users 'user1' \
  --check privileges \
  --output-format text

示例 2.1：比较全部用户，但排除系统账号

go run ./cmd/mysqlcompare \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn '10.0.0.11' \
  --target-dsn '10.0.0.12' \
  --source-schemas 'db0' \
  --target-schemas 'db*' \
  --exclude-users 'mysql.session,mysql.sys' \
  --check privileges \
  --output-format text

示例 3：只比较结构

go run ./cmd/mysqlcompare \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn '10.0.0.11' \
  --target-dsn '10.0.0.12' \
  --source-schemas 'db0' \
  --target-schemas 'db1' \
  --check structure \
  --output-format text


7. 结构对比逻辑

结构对比按 schema pair 执行，即每一对源库和目标库分别比较。

结构对比内容包括：
- 表是否存在
- 列定义
- 索引定义
- 部分表选项

结构差异包括：
- 源有目标没有的表
- 目标有源没有的表
- 同名表的列差异
- 同名表的索引差异
- 同名表的表选项差异

当前会忽略的典型差异：
- AUTO_INCREMENT 值
- 整数显示宽度差异
- 某些 extra 标准化后的无意义差异


8. schema pair 配对逻辑

工具会先根据源端和目标端的库选择结果生成 schema pair，后续结构对比和部分权限对比都依赖这个配对结果。

常见规则：

1. 源端精确选中单库，目标端选中多个库
   会生成：
   source_db -> target_db1
   source_db -> target_db2

2. 目标端精确选中单库，源端选中多个库
   会生成：
   source_db1 -> target_db
   source_db2 -> target_db

3. 源端和目标端都精确选中多个库，且数量一致
   按顺序一一配对

4. 如果不满足上面条件
   则退化为按同名库取交集配对

如果最终没有形成任何 schema pair：
- 结构对比不会产生库级配对结果
- 权限对比中的库级、表级权限不会参与比较


9. 权限对比逻辑

权限对比按用户进行。

权限分为三类：
- 全局权限
- 库级权限
- 表级权限

三类权限的判定规则不同。


10. 全局权限对比规则

全局权限不依赖 schema pair。

只比较同一个用户在源端和目标端的全局权限集合是否一致。

例如：
- GRANT SELECT ON *.* TO user1
- GRANT INSERT ON *.* TO user1

这类权限不和具体数据库名绑定。

如果一个用户只有全局权限，没有任何库级、表级权限：
- 即使没有 schema pair
- 仍然会正常参与比较


11. 库级权限对比规则

库级权限必须依赖 schema pair。

只比较映射后的库权限，不会把目标端其他库上的权限拿来替代。

例如：
- 源端选中 db0
- 目标端最终匹配出 db1
- schema pair 为：
  db0 -> db1

那么 user1 的库级权限比较规则是：
- 比较源端 user1 在 db0 上的权限
- 对比目标端 user1 在 db1 上的权限

若两者一致，则认为库级权限一致。
若目标端 user1 在 db1 上没有权限，或者权限集合不同，则认为不一致。

注意：
- 即使目标端 user1 在其他库上有权限，例如 db0
- 也不能替代 db1 的比较结果
- 因为当前有效 pair 是 db0 -> db1，不是 db0 -> db0


12. 表级权限对比规则

表级权限同样必须依赖 schema pair。

例如 schema pair 为：
db0 -> db1

则只比较：
- 源端 user1 在 db0.orders 上的权限
- 目标端 user1 在 db1.orders 上的权限

不会使用目标端其他库下的同名表权限替代。


13. 无 schema pair 时的权限行为

如果没有形成任何 schema pair，工具的权限对比规则如下：

- 全局权限：继续比较
- 库级权限：忽略，不参与比较
- 表级权限：忽略，不参与比较

这样设计的原因是：
- 全局权限不依赖数据库名
- 库级和表级权限必须依赖明确的源库到目标库映射

因此，无 schema pair 时，不会再把“空 scope”解释为“比较用户所有库权限”。

这可以避免以下误判：

- 源端 user1 在 db0 上有权限
- 目标端实际匹配库应为 db1
- 但因为没有形成有效 schema pair，程序错误地拿目标端 db0 上残留的授权记录来比较
- 从而误判为一致

现在该场景下：
- 不会比较目标端其他库上的库级/表级权限
- 只会比较全局权限


14. 用户匹配模式

如果 --users 不传：
- 默认比较全部用户

如果同时传了 --exclude-users：
- 先选中全部用户
- 再排除命中的用户

--user-match-mode=user_host

- 按 user@host 精确区分身份
- user1@%
- user1@10.%
  视为两个不同身份

--user-match-mode=user

- 只按用户名比较
- 不同 host 的同名用户会合并成一个逻辑用户
- host 差异本身不会作为权限差异输出


15. 输出结果说明

text 输出中，每个目标端会输出：
- Target
- Status
- Schema pairs
- Structure diff
- Privilege diff

状态含义：
- consistent
  没有发现差异

- inconsistent
  发现了结构或权限差异

- failed
  本次目标端比较执行失败


16. 退出码

- 0
  所有目标都执行成功，且没有阻塞性差异

- 1
  比较执行成功，但存在阻塞性差异

- 2
  至少一个目标执行失败

权限差异中：
- 源有目标没有
- 或源目标同一用户权限内容不同

会被视为阻塞性差异。

仅目标多出的用户权限，仍会记为差异，但不会导致退出码变为 1。


17. 使用建议

1. 如果源库和目标库不是同名，建议显式指定 --source-schemas 和 --target-schemas，确保生成符合预期的 schema pair。
2. 编写目标库选择器时，使用 glob 语法，不要使用 MySQL LIKE 的 % 和 _。
3. 如果重点检查权限映射关系，建议同时观察输出中的 Schema pairs，确认实际比较的是哪一对库。
4. 当用户只有全局权限时，不需要关注 schema pair；当用户涉及库级、表级权限时，必须先确认 schema pair 是否正确。
