# insight-create-dbmgr 使用说明

## 功能说明

`insight-create-dbmgr` 用于在指定 GoldenDB 集群中创建管理员用户并执行授权。

流程：

1. 根据 `cluster_name` 查询 `clusterId`
2. 解析密码明文或 base64
3. 读取授权文件中的 `grantList`
4. 调用 `/open_api/insight/external/addDBUserAndGrant`

命令入口：

```bash
go run ./cmd/insight-create-dbmgr --help
```

## 参数说明

| 参数 | 说明 |
| --- | --- |
| `--api` | Insight 地址，必填 |
| `--cluster-name` | 集群名称，必填 |
| `--user-name` | 用户名，默认 `dbmgr` |
| `--user-host` | 客户端 host，默认 `%` |
| `--password` | 密码明文 |
| `--password-b64` | 密码 base64 |
| `--grant-file` | 授权语句 JSON 文件，必填 |
| `--remarks` | 备注 |
| `--verify-ssl` | 启用 SSL 证书校验，默认关闭 |

约束：

- 必须提供 `--password` 或 `--password-b64`
- `grant-file` 必须是以下两种格式之一：
  - JSON 字符串数组
  - 包含 `grantList` 字段的 JSON 对象

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
  --cluster-name prod_cluster_a \
  --user-name dbmgr \
  --user-host '%' \
  --password 'plain-password' \
  --grant-file ./grants.json
```

如果已准备好 base64 密码：

```bash
go run ./cmd/insight-create-dbmgr \
  --api 10.0.0.10:8444 \
  --cluster-name prod_cluster_a \
  --password-b64 c2VjcmV0 \
  --grant-file ./grants.json
```

## 输出结果

标准输出直接打印 Insight 接口返回的完整 JSON。

## 返回码

- `0`：接口返回 `code == 1`
- `1`：接口返回失败
- `2`：参数错误或请求失败
