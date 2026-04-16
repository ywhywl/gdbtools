from __future__ import annotations

import argparse
import sys
from typing import Dict, List

from .collector import collect_privileges, collect_schema_snapshot, remap_privilege_bundles, resolve_schemas, resolve_users
from .config import CompareOptions, parse_connection_dsn, parse_target_dsns
from .diffing import diff_privileges, diff_schema, map_schema_pairs
from .models import ComparisonReport, SchemaSnapshot, TargetComparison
from .mysql_client import build_client
from .reporting import render_report
from .utils import split_multi_value


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Compare MySQL schema structure and privileges.")
    parser.add_argument("--source-dsn", required=True, help="Source connection DSN, for example mysql://user:pass@host:3306/")
    parser.add_argument(
        "--target-dsn",
        action="append",
        required=True,
        help="Target connection DSN. Can be repeated or contain comma, pipe, or newline separated DSNs.",
    )
    parser.add_argument("--default-user", help="Default MySQL username used when DSN omits the user part")
    parser.add_argument("--default-password", help="Default MySQL password used when DSN omits the password part")
    parser.add_argument("--source-schemas", action="append", default=[], help="Source schema selectors")
    parser.add_argument("--target-schemas", action="append", default=[], help="Target schema selectors")
    parser.add_argument("--exclude-schemas", action="append", default=[], help="Schema selectors to exclude")
    parser.add_argument("--users", action="append", default=[], help="User selectors to compare")
    parser.add_argument("--exclude-users", action="append", default=[], help="User selectors to exclude")
    parser.add_argument(
        "--user-match-mode",
        choices=["user", "user_host"],
        default="user_host",
        help="Privilege comparison mode",
    )
    parser.add_argument(
        "--skip-structure",
        action="store_true",
        help="Skip table structure comparison",
    )
    parser.add_argument(
        "--skip-privileges",
        action="store_true",
        help="Skip privilege comparison",
    )
    parser.add_argument(
        "--output-format",
        choices=["text", "json"],
        default="text",
        help="Output format",
    )
    return parser


def parse_args(argv: List[str]) -> CompareOptions:
    parser = build_parser()
    args = parser.parse_args(argv)

    compare_structure = not args.skip_structure
    compare_privileges = not args.skip_privileges
    if not compare_structure and not compare_privileges:
        parser.error("At least one of structure or privilege comparison must be enabled")

    return CompareOptions(
        source=parse_connection_dsn(
            args.source_dsn,
            default_user=args.default_user,
            default_password=args.default_password,
        ),
        targets=parse_target_dsns(
            args.target_dsn,
            default_user=args.default_user,
            default_password=args.default_password,
        ),
        source_schemas=split_multi_value(args.source_schemas),
        target_schemas=split_multi_value(args.target_schemas),
        exclude_schemas=split_multi_value(args.exclude_schemas),
        users=split_multi_value(args.users),
        exclude_users=split_multi_value(args.exclude_users),
        user_match_mode=args.user_match_mode,
        compare_structure=compare_structure,
        compare_privileges=compare_privileges,
        output_format=args.output_format,
    )


def main(argv: List[str] | None = None) -> int:
    options = parse_args(argv or sys.argv[1:])

    source_client = build_client(options.source)
    target_clients = [build_client(target) for target in options.targets]

    source_available_schemas = resolve_schemas(source_client, [], options.exclude_schemas)
    source_selected_schemas = resolve_schemas(source_client, options.source_schemas, options.exclude_schemas)

    if not options.source_schemas:
        source_selected_schemas = source_available_schemas

    source_schema_cache: Dict[str, SchemaSnapshot] = {}
    source_users = resolve_users(source_client, options.users, options.exclude_users)

    comparisons: List[TargetComparison] = []
    for target, target_client in zip(options.targets, target_clients):
        target_available_schemas = resolve_schemas(target_client, [], options.exclude_schemas)
        target_selected_schemas = resolve_schemas(target_client, options.target_schemas, options.exclude_schemas)
        if not options.target_schemas:
            target_selected_schemas = target_available_schemas

        schema_pairs = map_schema_pairs(
            source_available=source_available_schemas,
            source_selected=source_selected_schemas,
            source_selectors=options.source_schemas,
            target_available=target_available_schemas,
            target_selected=target_selected_schemas,
            target_selectors=options.target_schemas,
        )

        schema_diffs = []
        if options.compare_structure:
            target_schema_cache: Dict[str, SchemaSnapshot] = {}
            for pair in schema_pairs:
                if pair.source_schema not in source_schema_cache:
                    source_schema_cache[pair.source_schema] = collect_schema_snapshot(source_client, pair.source_schema)
                if pair.target_schema not in target_schema_cache:
                    target_schema_cache[pair.target_schema] = collect_schema_snapshot(target_client, pair.target_schema)
                schema_diff = diff_schema(source_schema_cache[pair.source_schema], target_schema_cache[pair.target_schema])
                if schema_diff.has_changes():
                    schema_diffs.append(schema_diff)

        privilege_diff = diff_privileges({}, {})
        if options.compare_privileges:
            target_users = resolve_users(target_client, options.users, options.exclude_users)
            source_scope = sorted({pair.source_schema for pair in schema_pairs})
            target_scope = sorted({pair.target_schema for pair in schema_pairs})
            source_privileges = collect_privileges(source_client, source_users, options.user_match_mode, source_scope)
            target_privileges = collect_privileges(target_client, target_users, options.user_match_mode, target_scope)
            if schema_pairs:
                target_privileges = remap_privilege_bundles(
                    target_privileges,
                    {pair.target_schema: pair.source_schema for pair in schema_pairs},
                )
            privilege_diff = diff_privileges(source_privileges, target_privileges)

        comparisons.append(
            TargetComparison(
                target=target.display_name,
                schema_pairs=schema_pairs,
                schema_diffs=schema_diffs,
                privilege_diff=privilege_diff,
            )
        )

    report = ComparisonReport(source=options.source.display_name, comparisons=comparisons)
    print(render_report(report, options.output_format))
    return 0
