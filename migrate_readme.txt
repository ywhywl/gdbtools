mysql_migrate shell 工具使用说明

1. 工具说明

mysql_migrate.sh 是一个基于 mysqldump 和 mysql 的 MySQL 结构与数据迁移脚本。

支持能力：
- 使用 mysqldump 导出表结构
- 可选导出表数据
- 使用 mysql 导入到一个或多个目标库
- 支持一对多迁移
- 支持源 schema 与目标 schema 同名迁移
- 支持单个源 schema 迁移到多个不同名字的目标 schema
- 支持源侧中转服务器导出
- 支持目标侧中转服务器导入
- 当导出文件大于阈值时自动压缩再传输

默认行为：
- 默认迁移全部表结构
- 默认不迁移数据
- 默认数据 dump 不带 DROP 语句
- 源中转服务器为空时，默认使用执行脚本的服务器
- 目标中转服务器为空时，默认使用执行脚本的服务器


2. 脚本位置

脚本文件：
- scripts/mysql_migrate.sh

运行方式：

bash scripts/mysql_migrate.sh --help


3. 连接与认证

3.1 源库参数

- --source <host:port>
  源实例地址，必填
  示例：
  10.1.1.10:3306

- --source-schemas <schema1,schema2,...>
  源 schema 列表，必填
  支持多个 schema，逗号分隔

3.2 目标库参数

- --target <targets>
  目标库列表，必填
  单个目标格式：
  host:port:schema

  支持多个目标，多个目标之间可以用以下任一分隔：
  - 换行符
  - |
  - ,

  同时也支持多次传入 --target

示例：

--target 10.2.1.10:3306:app_prod

--target '10.2.1.10:3306:app_prod|10.2.1.11:3306:app_prod'

--target $'10.2.1.10:3306:app_prod\n10.2.1.11:3306:app_prod'

--target '10.2.1.10:3306:app_prod,10.2.1.11:3306:app_prod'

--target '10.2.1.10:3306:app_prod|10.2.1.11:3306:app_prod' \
--target '10.2.1.12:3306:app_prod'

3.3 认证参数

脚本默认不要求显式传用户名和密码。

支持的认证方式：
- login-path
- defaults-file
- MySQL 客户端默认读取的配置文件，例如：
  - /etc/my.cnf
  - ~/.my.cnf

参数：

- --source-login-path <name>
  源端导出使用的 login-path

- --target-login-path <name>
  目标端导入使用的 login-path

- --defaults-file <file>
  源和目标共用的 MySQL 配置文件

- --source-defaults-file <file>
  仅源端使用的 MySQL 配置文件

- --target-defaults-file <file>
  仅目标端使用的 MySQL 配置文件

优先级：
1. 源端优先使用 --source-login-path，其次 --source-defaults-file，其次 --defaults-file
2. 目标端优先使用 --target-login-path，其次 --target-defaults-file，其次 --defaults-file
3. 如果都未指定，则由 mysqldump/mysql 自行读取默认配置


4. 迁移内容控制

- --with-data
  是否迁移数据
  默认不迁移

- --data-tables <table1,table2,...>
  指定导出数据的表
  仅在 --with-data 生效时使用

  语义：
  - 不指定：表示不做表级限制，导出全部表数据
  - 指定为空字符串：和不指定效果一致
  - 指定表名列表：只导出这些表的数据

- --dump-data-with-drop
  数据导出时包含 DROP TABLE 语句
  默认不带

  说明：
  为了让数据导入仍然可执行，开启该参数时，数据 dump 会包含目标表的建表语句和数据，而不是纯数据 INSERT

- --structure-with-drop
  结构导出时包含 DROP TABLE 语句
  默认不带

- --no-create-schema
  禁止自动创建目标 schema

- --create-schema-if-missing
  目标 schema 不存在时自动创建
  默认开启

迁移语义：
- 默认只迁移结构
- 指定 --with-data 且未指定 --data-tables 时，迁移全部表数据
- 指定 --with-data --data-tables orders,items 时，只迁移指定表数据


5. schema 规则

规则 1：
- 源可以指定多个 schema

规则 2：
- 当目标 schema 名与源 schema 名不一致时，源只能指定一个 schema

规则 3：
- 当源指定多个 schema 时，目标 schema 必须与源 schema 同名

规则 4：
- 当目标 schema 与源 schema 不一致时，导入结构前会先在目标创建目标 schema

规则 5：
- 一个源 schema 可以对应多个目标

多 schema 同名迁移示例：

--source-schemas app1,app2 \
--target '10.2.1.10:3306:app1|10.2.1.10:3306:app2|10.2.1.11:3306:app1|10.2.1.11:3306:app2'

单 schema 改名迁移示例：

--source-schemas app_prod \
--target '10.2.1.10:3306:app_test_a|10.2.1.11:3306:app_test_b'


6. 中转服务器

参数：

- --source-relay <host>
  源侧中转服务器
  导出在该服务器执行
  不指定时默认当前执行机

- --target-relay <host>
  目标侧中转服务器
  导入在该服务器执行
  不指定时默认当前执行机

- --source-relay-user <user>
  源中转 SSH 用户

- --target-relay-user <user>
  目标中转 SSH 用户

- --relay-tmp-dir <dir>
  中转临时目录
  默认：
  /tmp/mysql_migrate

- --compress-threshold-mb <num>
  导出文件大于该大小后自动压缩
  默认：
  50

- --compress-cmd <cmd>
  压缩命令
  默认：
  gzip

- --decompress-cmd <cmd>
  解压命令
  默认：
  gzip -dc

执行流程：
1. 在源侧中转服务器执行导出
2. 如果导出文件大于阈值，则在源侧中转服务器压缩
3. 将导出文件取回到当前执行机
4. 再从当前执行机传输到目标侧中转服务器
5. 在目标侧中转服务器执行导入

前提：
- 当前执行机到中转服务器已配置好 SSH 免密


7. 基础参数

- --config <file>
  配置文件路径，可选

- --mysqldump-bin <path>
  mysqldump 命令路径
  默认：
  mysqldump

- --mysql-bin <path>
  mysql 命令路径
  默认：
  mysql

- --ssh-bin <path>
  ssh 命令路径
  默认：
  ssh

- --scp-bin <path>
  scp 命令路径
  默认：
  scp


8. 执行控制参数

- --dry-run
  只打印将执行的命令，不实际执行

- --verbose
  输出详细日志

- --keep-dump-files
  保留临时导出文件

- --cleanup
  执行完成后清理临时文件
  默认开启


9. 配置文件

--config 指向一个 shell 变量配置文件。

示例：

SOURCE="10.1.1.10:3306"
SOURCE_SCHEMAS="app_prod"
TARGETS=$'10.2.1.10:3306:app_prod\n10.2.1.11:3306:app_prod'
SOURCE_LOGIN_PATH="src_prod"
TARGET_LOGIN_PATH="dst_prod"
SOURCE_RELAY="relay-src.example.com"
TARGET_RELAY="relay-dst.example.com"
WITH_DATA="true"
DATA_TABLES="orders,order_items"
COMPRESS_THRESHOLD_MB="50"
VERBOSE="true"

说明：
- 配置文件会被脚本 source 执行
- 应只使用受信任的配置文件
- 命令行参数优先级高于配置文件


10. 参数约束

必填参数：
- --source
- --source-schemas
- --target

默认行为：
- 默认迁移全部表结构
- 默认不迁移数据
- 默认数据 dump 不带 DROP
- 默认自动创建缺失的目标 schema

约束：
- --with-data 未指定时，--data-tables 不生效
- --data-tables 不指定和传空值，含义相同
- --target 可重复传，也可单次传多个
- 每个目标必须满足 host:port:schema 格式
- 如果任一目标 schema 与源 schema 不同，则 --source-schemas 只能指定一个 schema


11. 示例命令

示例 1：只迁结构

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app_prod \
  --target '10.2.1.10:3306:app_prod|10.2.1.11:3306:app_prod'

示例 2：迁结构和全部数据

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app_prod \
  --target '10.2.1.10:3306:app_prod,10.2.1.11:3306:app_prod' \
  --with-data

示例 3：迁指定表数据

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app_prod \
  --target 10.2.1.10:3306:app_prod \
  --with-data \
  --data-tables orders,order_items

示例 4：单 schema 改名迁移

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app_prod \
  --target '10.2.1.10:3306:app_test_a|10.2.1.11:3306:app_test_b' \
  --with-data

示例 5：使用 login-path 和中转机

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app_prod \
  --target $'10.2.1.10:3306:app_prod\n10.2.1.11:3306:app_prod' \
  --source-login-path src_prod \
  --target-login-path dst_prod \
  --source-relay relay-src.example.com \
  --target-relay relay-dst.example.com

示例 6：数据 dump 带 DROP

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app_prod \
  --target 10.2.1.10:3306:app_prod \
  --with-data \
  --dump-data-with-drop


12. 非法示例

非法示例 1：

bash scripts/mysql_migrate.sh \
  --source 10.1.1.10:3306 \
  --source-schemas app1,app2 \
  --target '10.2.1.10:3306:appx|10.2.1.11:3306:appy'

原因：
- 目标 schema 与源 schema 不一致时，源只能指定一个 schema

非法示例 2：

--target 10.2.1.10:app_prod

原因：
- 目标格式错误，缺少 port
