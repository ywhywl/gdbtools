#!/usr/bin/env python3
from __future__ import annotations

import argparse
import base64
import json
import sys

from lib.insight_utils import configure_logging, normalize_api_base, request_json, search_cluster_id

logger = configure_logging()


def resolve_password_b64(password: str, password_b64: str) -> str:
    if password_b64.strip():
        return password_b64.strip()
    if not password:
        raise ValueError("必须提供 --password 或 --password-b64")
    return base64.b64encode(password.encode("utf-8")).decode("utf-8")


def load_grants(path: str) -> list[str]:
    with open(path, encoding="utf-8") as file_obj:
        data = json.load(file_obj)
    if isinstance(data, list):
        grants = [str(item).strip() for item in data if str(item).strip()]
    elif isinstance(data, dict) and isinstance(data.get("grantList"), list):
        grants = [str(item).strip() for item in data["grantList"] if str(item).strip()]
    else:
        raise ValueError("grant 文件必须是字符串数组，或包含 grantList 数组的对象")
    if not grants:
        raise ValueError("grant 文件不能为空")
    return grants


def main() -> int:
    parser = argparse.ArgumentParser(description="GoldenDB 新建管理员用户脚本")
    parser.add_argument("--api", required=True, help="Insight 地址，格式 host:port 或完整 URL")
    parser.add_argument("--cluster-name", required=True, help="集群名称")
    parser.add_argument("--user-name", default="dbmgr", help="用户名，默认 dbmgr")
    parser.add_argument("--user-host", default="%", help="客户端 host，默认 %%")
    parser.add_argument("--password", default="", help="密码明文")
    parser.add_argument("--password-b64", default="", help="密码 base64")
    parser.add_argument("--grant-file", required=True, help="授权语句 JSON 文件")
    parser.add_argument("--remarks", default="", help="备注")
    parser.add_argument("--verify-ssl", action="store_true", help="启用 SSL 证书校验；默认关闭")
    args = parser.parse_args()

    api_base = normalize_api_base(args.api)
    no_verify = not args.verify_ssl
    cluster_id = search_cluster_id(api_base, args.cluster_name, no_verify=no_verify)
    password_b64 = resolve_password_b64(args.password, args.password_b64)
    grant_list = load_grants(args.grant_file)

    payload = [{
        "clusterId": cluster_id,
        "userName": args.user_name,
        "userHost": args.user_host,
        "userPasswd": password_b64,
        "grantList": grant_list,
        "remarks": args.remarks,
    }]

    resp = request_json(
        f"{api_base}/open_api/insight/external/addDBUserAndGrant",
        "POST",
        body=payload,
        no_verify=no_verify,
    )
    print(json.dumps(resp, ensure_ascii=False, indent=2))
    return 0 if resp.get("code") == 1 else 1


if __name__ == "__main__":
    sys.exit(main())
