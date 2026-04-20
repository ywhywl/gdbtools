# GoldenDB 统一接口操作客户端

客户端实现位置：
- [pkg/goldendb](/Users/wenlongy/dev/src/gdbtools/pkg/goldendb)

## 能力

- 加载工具接口注册表 YAML
- 自动注入 `username` / `password(base64)` 鉴权头
- 按 `tool name` 调用接口
- 对需要 `clusterId` 的请求，支持直接传 `clusterName`
- `clusterName` 按精确值匹配；若匹配到多个同名集群会直接报错
- 仍保留直接传 `clusterId` 的能力
- 根据 `async_pairs` 做 `taskId` 查询
- 提供轮询接口结果的统一入口

## 最小示例

```go
package main

import (
	"context"
	"log"
	"path/filepath"

	"gdbtools/pkg/goldendb"
)

func main() {
	registry, err := goldendb.LoadRegistry(filepath.Join("docs", "goldendb_tool_interface_set.yaml"))
	if err != nil {
		log.Fatal(err)
	}

	client, err := goldendb.NewClient(goldendb.ClientOptions{
		BaseURL: "https://127.0.0.1:8444",
		Auth: goldendb.Auth{
			Username: "admin",
			Password: "plain-text-password",
		},
		Registry: registry,
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.InvokeTool(context.Background(), "rest_tree_list", goldendb.InvokeInput{})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("status=%d code=%d msg=%s", result.StatusCode, result.Response.Code, result.Response.Msg)
}
```

## 按工具名执行

```go
result, err := client.InvokeTool(ctx, "tenancy_query_table_ddl", goldendb.InvokeInput{
	Query: map[string]any{
		"clusterName": "cluster-prod-a",
		"dbName":      "appdb",
		"tableName":   "orders",
	},
})
```

客户端会先调用 `tenancy_query_clusters` 获取集群清单，再把 `clusterName` 精准转换成 `clusterId` 后发起真实请求。

## 异步任务查询

如果某个动作工具在注册表里存在 `async_pairs` 映射，可以直接查询：

```go
result, err := client.QueryTask(ctx, "tenancy_create", "task-123")
```

## 异步任务轮询

```go
result, err := client.PollTask(ctx, "tenancy_create", "task-123", goldendb.PollOptions{
	MaxTries:  20,
	Interval:  3 * time.Second,
	Stop: func(result *goldendb.InvokeResult) (bool, error) {
		return result.Response != nil && result.Response.Code == 1, nil
	},
})
```

## 设计说明

- 客户端不把文档里的所有请求字段硬编码死。
- 业务接口通过注册表做 `tool name -> method/path` 映射。
- 具体字段校验和展示，建议仍优先使用：
  - `rest_tree_list`
  - `rest_info_get`

这样接口升级时不需要同步改大量调用代码。
