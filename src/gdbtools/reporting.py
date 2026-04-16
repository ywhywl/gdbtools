from __future__ import annotations

import json
from typing import List

from .models import ComparisonReport, SchemaDiff, TableDiff


def render_report(report: ComparisonReport, output_format: str) -> str:
    if output_format == "json":
        return json.dumps(report.to_dict(), ensure_ascii=False, indent=2, sort_keys=True)
    return render_text_report(report)


def render_text_report(report: ComparisonReport) -> str:
    lines: List[str] = [f"Source: {report.source}"]
    for comparison in report.comparisons:
        lines.append(f"Target: {comparison.target}")
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
    return "\n".join(lines)


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
