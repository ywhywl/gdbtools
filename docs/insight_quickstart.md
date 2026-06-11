# Insight 最小操作手册

## 适用场景

这份手册给交付同学直接使用，只保留最常见的 5 个操作：

1. 主机纳管
2. 建集群
3. 初始化管理员用户
4. 扩容 CN
5. 扩容 DN

如果需要看字段说明、返回结构或高级参数，去看总览文档：

- [insight_tools_overview.md](insight_tools_overview.md)

## 一、主机纳管

输入文件 `hosts.csv` 示例：

```csv
room_name,server_ip,install_path,data_path,system_parameter,region
room_a,10.0.0.21,/data/gdb,/data/gdb/data,1,shanghai
room_a,10.0.0.22,/data/gdb,/data/gdb/data,1,shanghai
```

执行命令：

```bash
go run ./cmd/insight-batch-onboard-hosts \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --input ./hosts.csv \
  --ssh-user root \
  --ssh-password 'your-password' \
  --batch-size 5 \
  --output-json
```

用途：

- 先把服务器纳入 Insight 管理

## 二、建集群

输入文件 `clusters.csv` 示例：

```csv
num,cluster_name,cluster_group_name,M,S,TS,LS,OS,server_type
1,prod_cluster_a,group_a,10.0.0.21,10.0.0.22,,,10.0.0.23,vm_l
```

执行命令：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ins-user-pwd 'your-db-password' \
  --wait-completion \
  --output ./batch_create_result.json
```

只看请求体、不真正执行：

```bash
go run ./cmd/insight-batch-create \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --csv ./clusters.csv \
  --ins-user-pwd 'your-db-password' \
  --dry-run
```

用途：

- 按标准模板批量建 GoldenDB 集群

## 三、初始化管理员用户

授权文件 `grants.json` 示例：

```json
{
  "grantList": [
    "grant all privileges on *.* to 'dbmgr'@'%'",
    "flush privileges"
  ]
}
```

执行命令：

```bash
go run ./cmd/insight-create-dbmgr \
  --api 10.0.0.10:8444 \
  --insight-user admin \
  --insight-password 'insight-password' \
  --cluster-name prod_cluster_a \
  --user-name dbmgr \
  --user-host '%' \
  --db-user-password 'your-db-password' \
  --grant-file ./grants.json
```

用途：

- 给新集群初始化管理账号

## 四、扩容 CN

输入文件 `batch_add_cn.csv` 示例：

```csv
insight_addr,cluster_name,template_name,ip,port,install_user,install_path,service_port
10.0.0.10:8444,prod_cluster_a,template_vm_l_cn,10.0.0.31,0,nudbproxy1,/data/goldendb/nudbproxy1,3306
10.0.0.10:8444,prod_cluster_a,template_vm_l_cn,10.0.0.32,0,nudbproxy2,/data/goldendb/nudbproxy2,3307
```

执行命令：

```bash
go run ./cmd/insight-batch-add-cn \
  --input ./batch_add_cn.csv \
  --insight-user admin \
  --insight-password 'insight-password' \
  --default-port 0 \
  --poll-interval 5 \
  --output-json
```

用途：

- 给已有集群增加 CN 节点

## 五、扩容 DN

输入文件 `batch_add_dn.csv` 示例：

```csv
insight_addr,cluster_name,template_name,dbgroup_name,team_id,ip,port,admin_port,install_user,install_path,data_path,log_path
10.0.0.10:8444,prod_cluster_a,template_vm_l_dn,group_1,1,10.0.0.41,0,5501,nudb1,/data/goldendb/nudb1,/data/goldendb/nudb1/data,/data/goldendb/nudb1/log
```

执行命令：

```bash
go run ./cmd/insight-batch-add-dn \
  --input ./batch_add_dn.csv \
  --insight-user admin \
  --insight-password 'insight-password' \
  --default-admin-port 5501 \
  --output-json
```

用途：

- 给已有集群增加 DN 节点

## 推荐执行顺序

新环境交付时，通常按这个顺序执行：

1. `insight-batch-onboard-hosts`
2. `insight-batch-create`
3. `insight-create-dbmgr`

扩容时按这个顺序选择：

1. 只扩计算节点：`insight-batch-add-cn`
2. 只扩存储节点：`insight-batch-add-dn`

## 常见注意事项

- `api` 或 `insight_addr` 可以写成 `10.0.0.10:8444`
- 所有命令都必须提供 `--insight-user`
- 所有命令都必须提供 `--insight-password` 或 `--insight-password-b64`
- 非本地地址默认会优先走 `https`
- 如果目标环境证书不完整，可以加 `--no-verify` 或不要开启 `--verify-ssl`
- `batch-create` 必须提供 `--ins-user-pwd` 或 `--ins-user-pwd-base64`
- `create-dbmgr` 必须提供 `--db-user-password` 或 `--db-user-password-b64`
- `batch-add-dn` 必须提供 `dbgroup_name` 或 `dbgroup_id`

## 相关文档

- [insight_tools_overview.md](insight_tools_overview.md)
- [insight_batch_onboard_hosts.md](insight_batch_onboard_hosts.md)
- [insight_batch_create.md](insight_batch_create.md)
- [insight_create_dbmgr.md](insight_create_dbmgr.md)
- [insight_batch_add_cn.md](insight_batch_add_cn.md)
- [insight_batch_add_dn.md](insight_batch_add_dn.md)
