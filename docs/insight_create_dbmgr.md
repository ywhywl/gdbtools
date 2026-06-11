# insight-create-dbmgr 使用说明

## 功能说明

`insight-create-dbmgr` 用于在指定 GoldenDB 集群中创建管理员用户并执行授权。

流程：

1. 使用 Insight 登录用户名和密码作为请求头鉴权
2. 根据 `cluster_name` 查询 `clusterId`
3. 解析“新建数据库用户密码”明文、base64 或密码映射文件
4. 读取授权文件中的 `grantList`
5. 调用 `/open_api/insight/external/addDBUserAndGrant`

命令入口：

```bash
go run ./cmd/insight-create-dbmgr --help
```

## 参数说明

| 参数 | 说明 |
| --- | --- |
| `--api` | Insight 地址，必填 |
| `--insight-user` | Insight 登录用户名，必填 |
| `--insight-password` | Insight 登录密码明文 |
| `--insight-password-b64` | Insight 登录密码 base64 |
| `--cluster-name` | 集群名称，必填 |
| `--user-name` | 用户名，默认 `dbmgr` |
| `--user-host` | 客户端 host，默认 `%` |
| `--db-user-password` | 新建数据库用户密码明文 |
| `--db-user-password-b64` | 新建数据库用户密码 base64 |
| `--db-user-password-file` | 新建数据库用户密码映射文件(JSON) |
| `--grant-file` | 授权语句 JSON 文件，必填 |
| `--remarks` | 备注 |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |

约束：

- 必须提供 `--insight-user`
- 必须提供 `--insight-password` 或 `--insight-password-b64`
- 必须提供以下其中一种：
  - `--db-user-password`
  - `--db-user-password-b64`
  - `--db-user-password-file`
- `grant-file` 必须是以下两种格式之一：
  - JSON 字符串数组
  - 包含 `grantList` 字段的 JSON 对象

说明：

- Insight 鉴权信息放在请求头：
  - `username`: Insight 登录用户名
  - `password`: Insight 登录密码的 base64
- 新建数据库用户密码放在请求体字段 `userPasswd`
- `--db-user-password-file` 适用于一次创建多个用户
- 兼容旧参数 `--password` / `--password-b64`，其语义等同于数据库用户密码，但不建议继续使用

## 多用户密码映射文件格式

`--db-user-password-file` 要求传入 JSON 对象。

键格式：

- `user@host`

值支持两种形式。

形式一：直接写明文密码

```json
{
  "dbmgr@%": "plain-password",
  "myawr@%": "another-password"
}
```

形式二：对象形式，支持 `password`、`passwordB64` 和 `remarks`

```json
{
  "dbmgr@%": {
    "password": "plain-password",
    "remarks": "管理员用户"
  },
  "myawr@%": {
    "passwordB64": "YW5vdGhlci1wYXNzd29yZA==",
    "remarks": "只读用户"
  }
}
```

说明：

- 同一个映射文件会展开成接口的数组请求体
- `grant-file` 仍然是全局共用的
- 如果某个映射项配置了 `remarks`，会覆盖命令行的 `--remarks`

## grant 文件格式

格式一：

```json
[
  "grant all privileges on *.* to 'dbmgr'@'%'",
  "flush privileges"
]
```

格式二：

```json
{
  "grantList": [
    "grant all privileges on *.* to 'dbmgr'@'%'",
    "flush privileges"
  ]
}
```

## 使用示例

```bash
go run ./cmd/insight-create-dbmgr \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --cluster-name prod_cluster_a \
  --user-name dbmgr \
  --user-host '%' \
  --db-user-password 'plain-password' \
  --grant-file ./grants.json
```

如果已准备好 base64 密码：

```bash
go run ./cmd/insight-create-dbmgr \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password-b64 aW5zaWdodC1wYXNzd29yZA== \
  --cluster-name prod_cluster_a \
  --db-user-password-b64 c2VjcmV0 \
  --grant-file ./grants.json
```

使用密码映射文件批量创建多个用户：

```bash
go run ./cmd/insight-create-dbmgr \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --cluster-name prod_cluster_a \
  --db-user-password-file ./db_users.json \
  --grant-file ./grants.json
```

## 输出结果

标准输出直接打印 Insight 接口返回的完整 JSON。

## 返回码

- `0`：接口返回 `code == 1`
- `1`：接口返回失败
- `2`：参数错误或请求失败
