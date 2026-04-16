from __future__ import annotations

import json
from typing import List

from .models import ComparisonReport, ComparisonSummary, SchemaDiff, TableDiff, TargetComparison


def render_report(report: ComparisonReport, output_format: str) -> str:
    if output_format == "json":
        return json.dumps(report.to_dict(), ensure_ascii=False, indent=2, sort_keys=True)
    return render_text_report(report)


def render_text_report(report: ComparisonReport) -> str:
    lines: List[str] = [f"Source: {report.source}"]
    for comparison in report.comparisons:
        lines.extend(render_text_target(comparison))
    if report.summary:
        lines.extend(render_text_summary(report.summary))
    return "\n".join(lines)


def render_target_report(comparison: TargetComparison, output_format: str) -> str:
    if output_format == "json":
        payload = {
            "target": comparison.target,
            "status": "failed" if comparison.error else "success",
            "error": comparison.error,
            "schema_pairs": [pair.__dict__ for pair in comparison.schema_pairs],
            "schema_diffs": [
                {
                    "source_schema": diff.source_schema,
                    "target_schema": diff.target_schema,
                    "source_only_tables": diff.source_only_tables,
                    "target_only_tables": diff.target_only_tables,
                    "changed_tables": [table_diff.__dict__ for table_diff in diff.changed_tables],
                }
                for diff in comparison.schema_diffs
            ],
            "privilege_diff": comparison.privilege_diff.__dict__,
        }
        return json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True, default=str)
    return "\n".join(render_text_target(comparison))


def render_summary_report(summary: ComparisonSummary, output_format: str) -> str:
    if output_format == "json":
        return json.dumps({"summary": summary.__dict__}, ensure_ascii=False, indent=2, sort_keys=True)
    return "\n".join(render_text_summary(summary))


def render_text_target(comparison: TargetComparison) -> List[str]:
    lines: List[str] = [f"Target: {comparison.target}"]
    if comparison.error:
        lines.append(f"  Status: failed")
        lines.append(f"  Error: {comparison.error}")
        return lines

    lines.append(f"  Status: {'inconsistent' if comparison.has_differences() else 'consistent'}")
    if not comparison.schema_pairs:
        lines.append("  Schema pairs: none")
    else:
        lines.append("  Schema pairs:")
        for pair in comparison.schema_pairs:
            lines.append(f"    - {pair.source_schema} -> {pair.target_schema}")

    if not comparison.schema_diffs:
        lines.append("  Structure diff: no differences")
    else:
        lines.append("  Structure diff:")
        for schema_diff in comparison.schema_diffs:
            lines.extend(render_schema_diff(schema_diff))

    privilege_diff = comparison.privilege_diff
    if not privilege_diff.has_changes():
        lines.append("  Privilege diff: no differences")
    else:
        lines.append("  Privilege diff:")
        if privilege_diff.source_only_identities:
            lines.append("    Source only identities:")
            for identity in privilege_diff.source_only_identities:
                lines.append(f"      - {identity['identity']}")
        if privilege_diff.target_only_identities:
            lines.append("    Target only identities:")
            for identity in privilege_diff.target_only_identities:
                lines.append(f"      - {identity['identity']}")
        if privilege_diff.changed_identities:
            lines.append("    Changed identities:")
            for item in privilege_diff.changed_identities:
                lines.append(f"      - {item['identity']}")
    return lines


def render_text_summary(summary: ComparisonSummary) -> List[str]:
    return [
        "Summary:",
        f"  Total targets: {summary.total_targets}",
        f"  Successful targets: {summary.successful_targets}",
        f"  Failed targets: {summary.failed_targets}",
        f"  Consistent targets: {summary.consistent_targets}",
        f"  Inconsistent targets: {summary.inconsistent_targets}",
    ]


def render_schema_diff(schema_diff: SchemaDiff) -> List[str]:
    lines: List[str] = [f"    {schema_diff.source_schema} -> {schema_diff.target_schema}"]
    if not schema_diff.has_changes():
        lines.append("      no differences")
        return lines
    if schema_diff.source_only_tables:
        lines.append(f"      source only tables: {', '.join(schema_diff.source_only_tables)}")
    if schema_diff.target_only_tables:
        lines.append(f"      target only tables: {', '.join(schema_diff.target_only_tables)}")
    for table_diff in schema_diff.changed_tables:
        lines.extend(render_table_diff(table_diff))
    return lines


def render_table_diff(table_diff: TableDiff) -> List[str]:
    lines = [f"      table: {table_diff.table}"]
    for option, values in sorted(table_diff.changed_table_options.items()):
        lines.append(f"        option {option}: source={values['source']} target={values['target']}")
    for column in table_diff.source_only_columns:
        lines.append(f"        source only column: {column['name']}")
    for column in table_diff.target_only_columns:
        lines.append(f"        target only column: {column['name']}")
    for column in table_diff.changed_columns:
        lines.append(f"        changed column: {column['column']}")
    for index in table_diff.source_only_indexes:
        lines.append(f"        source only index: {index['name']}")
    for index in table_diff.target_only_indexes:
        lines.append(f"        target only index: {index['name']}")
    for index in table_diff.changed_indexes:
        lines.append(f"        changed index: {index['index']}")
    return lines
