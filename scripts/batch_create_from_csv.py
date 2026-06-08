#!/usr/bin/env python3
from __future__ import annotations

import argparse
import base64
import json
import sys
import time
from dataclasses import dataclass
from typing import Any

from lib.insight_utils import (
    configure_logging,
    normalize_api_base,
    parse_csv_input,
    request_json,
)

logger = configure_logging()

CREATE_CLUSTER_ENDPOINT = "/open_api/insight/external/tenant/createCluster"
QUERY_CREATE_CLUSTER_RESULT_ENDPOINT = "/open_api/insight/external/tenant/getInstallClusterProcess"
SUPPORTED_SERVER_TYPES = {"vm_l", "vm_m", "vm_h", "pm", "vm_lowercase_0"}
REQUIRED_HEADERS = [
    "num",
    "cluster_name",
    "cluster_group_name",
    "M",
    "S",
    "TS",
    "LS",
    "OS",
    "server_type",
]
ROLE_SEQUENCE = ("M", "S", "LS", "OS", "TS")
ROLE_TO_TEAM_ID = {
    "M": 1,
    "S": 2,
    "LS": 3,
    "OS": 4,
    "TS": 5,
}
ROLE_TO_DB_ROLE = {
    "M": 1,
    "S": 0,
    "TS": 0,
    "LS": 0,
    "OS": 2,
}


@dataclass
class TemplateSelection:
    server_type: str
    global_template: str
    dn_template: str
    cn_template: str
    os_cn_template: str

    def as_dict(self) -> dict[str, str]:
        return {
            "server_type": self.server_type,
            "global_template": self.global_template,
            "dn_template": self.dn_template,
            "cn_template": self.cn_template,
            "os_cn_template": self.os_cn_template,
        }


@dataclass
class NormalizedRow:
    row_no: int
    num: str
    cluster_name: str
    cluster_group_name: str
    role_ips: dict[str, str]
    server_type: str
    templates: TemplateSelection


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="GoldenDB 批量创建租户脚本")
    parser.add_argument("--api", required=True, help="Insight 地址，格式 host:port 或完整 URL")
    parser.add_argument("--csv", required=True, help="CSV 文件路径")
    parser.add_argument("--prefix", default="nu", help="安装用户名前缀，默认 nu")
    parser.add_argument("--base-path", default="/data/goldendb", help="安装根目录，默认 /data/goldendb")
    parser.add_argument("--ins-user-pwd", default="", help="业务用户密码明文")
    parser.add_argument("--ins-user-pwd-base64", default="", help="业务用户密码 base64")
    parser.add_argument("--ha-mode", type=int, default=0, help="高可用模式，默认 0")
    parser.add_argument("--instance-type", type=int, default=1, help="实例类型，默认 1")
    parser.add_argument("--charset", default="utf8mb4", help="字符集，默认 utf8mb4")
    parser.add_argument("--mode", type=int, default=1, help="安装模式，默认 1")
    parser.add_argument("--gtm-use-mode", type=int, default=1, help="GTM 使用模式，默认 1")
    parser.add_argument("--cluster-desc", default="", help="集群描述，默认取 cluster_name")
    parser.add_argument("--wait-completion", action="store_true", help="提交后等待任务完成")
    parser.add_argument("--max-wait-time", type=int, default=3600, help="最大等待秒数，默认 3600")
    parser.add_argument("--poll-interval", type=int, default=10, help="轮询间隔秒数，默认 10")
    parser.add_argument("--max-retries", type=int, default=1, help="失败重试次数，默认 1")
    parser.add_argument("--no-verify", dest="no_verify", action="store_true", default=True, help="跳过 SSL 证书校验，默认开启")
    parser.add_argument("--verify-ssl", dest="no_verify", action="store_false", help="启用 SSL 证书校验")
    parser.add_argument("--dry-run", action="store_true", help="只渲染请求体，不发起接口调用")
    parser.add_argument("--output", default="", help="将结构化结果写入 JSON 文件")
    parser.add_argument("--format", choices=("json", "text"), default="json", help="标准输出格式，默认 json")
    return parser.parse_args()


def resolve_password_b64(args: argparse.Namespace) -> str:
    password_b64 = args.ins_user_pwd_base64.strip()
    if password_b64:
        return password_b64
    password = args.ins_user_pwd
    if not password:
        raise ValueError("必须提供 --ins-user-pwd 或 --ins-user-pwd-base64")
    return base64.b64encode(password.encode("utf-8")).decode("utf-8")


def resolve_templates(server_type: str) -> TemplateSelection:
    normalized = str(server_type or "").strip()
    if normalized not in SUPPORTED_SERVER_TYPES:
        supported = ", ".join(sorted(SUPPORTED_SERVER_TYPES))
        raise ValueError(f"不支持的 server_type: {normalized}，当前仅支持 {supported}")
    return TemplateSelection(
        server_type=normalized,
        global_template=f"template_{normalized}_cluster",
        dn_template=f"template_{normalized}_dn",
        cn_template=f"template_{normalized}_cn",
        os_cn_template=f"template_{normalized}_cn_OS",
    )


def load_rows(path: str) -> list[NormalizedRow]:
    raw_rows = parse_csv_input(path)
    if not raw_rows:
        raise ValueError("CSV 文件为空")

    headers = {key for row in raw_rows for key in row.keys()}
    missing = [header for header in REQUIRED_HEADERS if header not in headers]
    if missing:
        raise ValueError(f"CSV 缺少必填列: {', '.join(missing)}")

    rows: list[NormalizedRow] = []
    cluster_names: set[str] = set()
    for index, row in enumerate(raw_rows, start=1):
        cluster_name = str(row.get("cluster_name", "") or "").strip()
        if not cluster_name:
            raise ValueError(f"第 {index} 行 cluster_name 不能为空")
        if cluster_name in cluster_names:
            raise ValueError(f"第 {index} 行 cluster_name 重复: {cluster_name}")
        cluster_names.add(cluster_name)

        role_ips = {role: str(row.get(role, "") or "").strip() for role in ROLE_SEQUENCE}
        if not role_ips["M"] or not role_ips["S"]:
            raise ValueError(f"第 {index} 行 M/S 不能为空")

        ip_seen: set[str] = set()
        for role, ip in role_ips.items():
            if not ip:
                continue
            if ip in ip_seen:
                raise ValueError(f"第 {index} 行 IP 重复: {ip}")
            ip_seen.add(ip)

        server_type = str(row.get("server_type", "") or "").strip()
        if not server_type:
            raise ValueError(f"第 {index} 行 server_type 不能为空")

        rows.append(NormalizedRow(
            row_no=index,
            num=str(row.get("num", "") or "").strip(),
            cluster_name=cluster_name,
            cluster_group_name=str(row.get("cluster_group_name", "") or "").strip(),
            role_ips=role_ips,
            server_type=server_type,
            templates=resolve_templates(server_type),
        ))
    return rows


def build_cn_install_list(row: NormalizedRow, args: argparse.Namespace) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    for role in ROLE_SEQUENCE:
        ip = row.role_ips[role]
        if not ip:
            continue
        template_name = row.templates.os_cn_template if role == "OS" else row.templates.cn_template
        for suffix, service_port in ((1, 3306), (2, 3307)):
            install_user = f"{args.prefix}dbproxy{suffix}"
            item: dict[str, Any] = {
                "ip": ip,
                "installPath": f"{args.base_path}/{install_user}",
                "installUser": install_user,
                "servicePort": service_port,
            }
            if role == "OS":
                item["templateName"] = template_name
            items.append(item)
    return items


def build_dn_install_list(row: NormalizedRow, args: argparse.Namespace) -> list[dict[str, Any]]:
    install_user = f"{args.prefix}db1"
    install_path = f"{args.base_path}/{install_user}"
    data_path = f"{install_path}/data"
    team_list: list[dict[str, Any]] = []
    for role in ROLE_SEQUENCE:
        ip = row.role_ips[role]
        if not ip:
            continue
        team_list.append({
            "teamId": ROLE_TO_TEAM_ID[role],
            "dnList": [{
                "ip": ip,
                "dbRole": ROLE_TO_DB_ROLE[role],
                "installPath": install_path,
                "installUser": install_user,
                "dataPath": data_path,
            }],
        })
    return [{
        "dbgroupId": 1,
        "teamList": team_list,
    }]


def build_payload(row: NormalizedRow, args: argparse.Namespace, password_b64: str) -> dict[str, Any]:
    cluster_desc = args.cluster_desc.strip() or row.cluster_name
    return {
        "configMode": 1,
        "clusterInstallInfo": {
            "mode": args.mode,
            "charset": args.charset,
            "dbgroupNum": 1,
            "insUserPwd": password_b64,
            "clusterDesc": cluster_desc,
            "clusterName": row.cluster_name,
            "instanceType": args.instance_type,
            "gtmUseMode": args.gtm_use_mode,
            "haMode": args.ha_mode,
        },
        "dnInstallList": build_dn_install_list(row, args),
        "cnInstallList": build_cn_install_list(row, args),
        "parameterTemplateInfos": [
            {"type": "DN", "templateName": row.templates.dn_template},
            {"type": "CN", "templateName": row.templates.cn_template},
            {"type": "GLOBAL", "templateName": row.templates.global_template},
        ],
    }


def start_create_cluster(api_base: str, payload: dict[str, Any], no_verify: bool) -> str:
    resp = request_json(f"{api_base}{CREATE_CLUSTER_ENDPOINT}", "POST", body=payload, no_verify=no_verify)
    if resp.get("code") != 1:
        raise RuntimeError(json.dumps(resp, ensure_ascii=False))
    data = resp.get("data")
    if isinstance(data, dict):
        task_id = data.get("task_id") or data.get("taskId") or data.get("id")
        if task_id:
            return str(task_id)
    raise RuntimeError(f"接口未返回 task_id: {json.dumps(resp, ensure_ascii=False)}")


def poll_create_cluster_progress(
    api_base: str,
    task_id: str,
    poll_interval: int,
    poll_timeout: int,
    no_verify: bool,
) -> dict[str, Any]:
    deadline = time.time() + poll_timeout
    url = f"{api_base}{QUERY_CREATE_CLUSTER_RESULT_ENDPOINT}?taskId={task_id}"
    last_data: dict[str, Any] = {}

    while time.time() < deadline:
        resp = request_json(url, "GET", no_verify=no_verify)
        if resp.get("code") != 1:
            raise RuntimeError(resp.get("msg") or json.dumps(resp, ensure_ascii=False))

        data = resp.get("data") or {}
        if not isinstance(data, dict):
            raise RuntimeError(f"查询安装进度返回格式异常: {json.dumps(resp, ensure_ascii=False)}")

        last_data = data
        result = str(data.get("result", "") or "").strip().lower()
        process = data.get("process")
        logger.info("taskId=%s progress=%s result=%s", task_id, process, result or "unknown")

        if result == "success":
            return data
        if result == "fail":
            error_code = data.get("errorCode")
            error_msg = str(data.get("errorMsg", "") or "").strip() or "install failed"
            raise RuntimeError(f"taskId={task_id} errorCode={error_code} errorMsg={error_msg}")

        time.sleep(poll_interval)

    raise RuntimeError(
        f"任务轮询超时: taskId={task_id}, last_process={last_data.get('process')}, last_result={last_data.get('result')}"
    )


def build_template_selection_output(row: NormalizedRow) -> dict[str, Any]:
    selection = row.templates.as_dict()
    if not row.role_ips.get("OS"):
        selection.pop("os_cn_template", None)
    return selection


def log_template_selection(rows: list[NormalizedRow]) -> None:
    for row in rows:
        selection = build_template_selection_output(row)
        logger.info(
            "cluster=%s template_selection=%s",
            row.cluster_name,
            json.dumps(selection, ensure_ascii=False),
        )


def execute_row(
    api_base: str,
    row: NormalizedRow,
    payload: dict[str, Any],
    args: argparse.Namespace,
) -> dict[str, Any]:
    last_error = ""
    task_id = ""
    response: dict[str, Any] | None = None
    retries = max(args.max_retries, 1)

    for attempt in range(1, retries + 1):
        try:
            task_id = start_create_cluster(api_base, payload, no_verify=args.no_verify)
            result: dict[str, Any] | None = None
            if args.wait_completion and task_id:
                result = poll_create_cluster_progress(
                    api_base,
                    task_id,
                    poll_interval=args.poll_interval,
                    poll_timeout=args.max_wait_time,
                    no_verify=args.no_verify,
                )
            return {
                "row_no": row.row_no,
                "num": row.num,
                "cluster_name": row.cluster_name,
                "cluster_group_name": row.cluster_group_name,
                "server_type": row.server_type,
                "template_selection": build_template_selection_output(row),
                "status": "success",
                "task_id": task_id,
                "attempt": attempt,
                "request_payload": payload,
                "response": result,
                "error": "",
            }
        except Exception as exc:
            last_error = str(exc)
            response = None
            logger.warning("集群 %s 第 %s/%s 次执行失败: %s", row.cluster_name, attempt, retries, exc)

    return {
        "row_no": row.row_no,
        "num": row.num,
        "cluster_name": row.cluster_name,
        "cluster_group_name": row.cluster_group_name,
        "server_type": row.server_type,
        "template_selection": build_template_selection_output(row),
        "status": "failed",
        "task_id": task_id,
        "attempt": retries,
        "request_payload": payload,
        "response": response,
        "error": last_error,
    }


def write_output(path: str, payload: dict[str, Any]) -> None:
    if not path:
        return
    with open(path, "w", encoding="utf-8") as file_obj:
        json.dump(payload, file_obj, ensure_ascii=False, indent=2)
        file_obj.write("\n")


def render_stdout(args: argparse.Namespace, output: dict[str, Any]) -> None:
    if args.format == "json":
        print(json.dumps(output, ensure_ascii=False, indent=2))
        return

    summary = output.get("summary", {})
    logger.info("总计 total=%s success=%s failed=%s", summary.get("total"), summary.get("success_count"), summary.get("failed_count"))
    for item in output.get("clusters", []):
        logger.info(
            "cluster=%s server_type=%s status=%s task_id=%s templates=%s error=%s",
            item.get("cluster_name"),
            item.get("server_type"),
            item.get("status"),
            item.get("task_id"),
            json.dumps(item.get("template_selection", {}), ensure_ascii=False),
            item.get("error"),
        )


def main() -> int:
    args = parse_args()
    api_base = normalize_api_base(args.api)
    password_b64 = resolve_password_b64(args)
    rows = load_rows(args.csv)

    if args.gtm_use_mode != 1:
        raise ValueError("第一版仅支持 --gtm-use-mode=1")

    log_template_selection(rows)

    clusters: list[dict[str, Any]] = []
    if args.dry_run:
        for row in rows:
            payload = build_payload(row, args, password_b64)
            clusters.append({
                "row_no": row.row_no,
                "num": row.num,
                "cluster_name": row.cluster_name,
                "cluster_group_name": row.cluster_group_name,
                "server_type": row.server_type,
                "template_selection": build_template_selection_output(row),
                "status": "dry_run",
                "task_id": "",
                "attempt": 0,
                "request_payload": payload,
                "response": None,
                "error": "",
            })
    else:
        for row in rows:
            payload = build_payload(row, args, password_b64)
            clusters.append(execute_row(api_base, row, payload, args))

    success_count = sum(1 for item in clusters if item["status"] in {"success", "dry_run"})
    failed_count = sum(1 for item in clusters if item["status"] == "failed")
    output = {
        "success": failed_count == 0,
        "api": api_base,
        "summary": {
            "total": len(clusters),
            "success_count": success_count,
            "failed_count": failed_count,
        },
        "clusters": clusters,
    }

    write_output(args.output, output)
    render_stdout(args, output)

    if args.dry_run:
        return 0
    if failed_count == 0:
        return 0
    if success_count > 0:
        return 1
    return 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        print(json.dumps({
            "success": False,
            "summary": {"total": 0, "success_count": 0, "failed_count": 0},
            "error": str(exc),
        }, ensure_ascii=False, indent=2))
        sys.exit(3)
