from __future__ import annotations

from typing import Dict, List, Sequence

from .models import PrivilegeBundle, PrivilegeDiff, SchemaDiff, SchemaPair, SchemaSnapshot, TableDiff


TABLE_OPTION_FIELDS = ["engine", "row_format", "table_collation", "create_options", "table_comment"]


def map_schema_pairs(
    source_available: Sequence[str],
    source_selected: Sequence[str],
    source_selectors: Sequence[str],
    target_available: Sequence[str],
    target_selected: Sequence[str],
    target_selectors: Sequence[str],
) -> List[SchemaPair]:
    if not source_selected or not target_selected:
        return []

    source_exact = source_selectors and all(selector in source_available for selector in source_selectors)
    target_exact = target_selectors and all(selector in target_available for selector in target_selectors)

    if not source_selectors:
        source_exact = False
    if not target_selectors:
        target_exact = False

    if source_exact and target_exact:
        if len(source_selected) == 1:
            return [SchemaPair(source_selected[0], target) for target in target_selected]
        if len(target_selected) == 1:
            return [SchemaPair(source_schema, target_selected[0]) for source_schema in source_selected]
        if len(source_selected) == len(target_selected):
            return [SchemaPair(source_schema, target_schema) for source_schema, target_schema in zip(source_selected, target_selected)]
        raise ValueError("Exact source and target schema counts are different and cannot be paired")

    if source_exact and len(source_selected) == 1:
        return [SchemaPair(source_selected[0], target) for target in target_selected]

    if target_exact and len(target_selected) == 1:
        return [SchemaPair(source_schema, target_selected[0]) for source_schema in source_selected]

    common = sorted(set(source_selected) & set(target_selected))
    return [SchemaPair(name, name) for name in common]


def diff_schema(source: SchemaSnapshot, target: SchemaSnapshot) -> SchemaDiff:
    source_tables = source.tables
    target_tables = target.tables

    source_only_tables = sorted(set(source_tables) - set(target_tables))
    target_only_tables = sorted(set(target_tables) - set(source_tables))
    changed_tables: List[TableDiff] = []

    for table_name in sorted(set(source_tables) & set(target_tables)):
        source_table = source_tables[table_name]
        target_table = target_tables[table_name]
        table_diff = TableDiff(table=table_name)

        for field_name in TABLE_OPTION_FIELDS:
            source_value = getattr(source_table, field_name)
            target_value = getattr(target_table, field_name)
            if source_value != target_value:
                table_diff.changed_table_options[field_name] = {
                    "source": source_value,
                    "target": target_value,
                }

        source_columns = {column.name: column for column in source_table.columns}
        target_columns = {column.name: column for column in target_table.columns}
        for column_name in sorted(set(source_columns) - set(target_columns)):
            table_diff.source_only_columns.append(column_to_dict(source_columns[column_name]))
        for column_name in sorted(set(target_columns) - set(source_columns)):
            table_diff.target_only_columns.append(column_to_dict(target_columns[column_name]))
        for column_name in sorted(set(source_columns) & set(target_columns)):
            source_column = column_to_dict(source_columns[column_name])
            target_column = column_to_dict(target_columns[column_name])
            if source_column != target_column:
                table_diff.changed_columns.append(
                    {
                        "column": column_name,
                        "source": source_column,
                        "target": target_column,
                    }
                )

        source_indexes = {index.name: index_to_dict(index) for index in source_table.indexes}
        target_indexes = {index.name: index_to_dict(index) for index in target_table.indexes}
        for index_name in sorted(set(source_indexes) - set(target_indexes)):
            table_diff.source_only_indexes.append(source_indexes[index_name])
        for index_name in sorted(set(target_indexes) - set(source_indexes)):
            table_diff.target_only_indexes.append(target_indexes[index_name])
        for index_name in sorted(set(source_indexes) & set(target_indexes)):
            if source_indexes[index_name] != target_indexes[index_name]:
                table_diff.changed_indexes.append(
                    {
                        "index": index_name,
                        "source": source_indexes[index_name],
                        "target": target_indexes[index_name],
                    }
                )

        if table_diff.has_changes():
            changed_tables.append(table_diff)

    return SchemaDiff(
        source_schema=source.name,
        target_schema=target.name,
        source_only_tables=source_only_tables,
        target_only_tables=target_only_tables,
        changed_tables=changed_tables,
    )


def diff_privileges(
    source_bundles: Dict[str, PrivilegeBundle],
    target_bundles: Dict[str, PrivilegeBundle],
) -> PrivilegeDiff:
    source_keys = set(source_bundles)
    target_keys = set(target_bundles)

    source_only_identities = [source_bundles[key].to_dict() for key in sorted(source_keys - target_keys)]
    target_only_identities = [target_bundles[key].to_dict() for key in sorted(target_keys - source_keys)]
    changed_identities = []

    for key in sorted(source_keys & target_keys):
        source_payload = source_bundles[key].to_dict()
        target_payload = target_bundles[key].to_dict()
        if source_payload != target_payload:
            changed_identities.append(
                {
                    "identity": source_bundles[key].identity.display_name,
                    "source": source_payload,
                    "target": target_payload,
                }
            )

    return PrivilegeDiff(
        source_only_identities=source_only_identities,
        target_only_identities=target_only_identities,
        changed_identities=changed_identities,
    )


def column_to_dict(column: object) -> Dict[str, object]:
    return {
        "ordinal_position": column.ordinal_position,
        "name": column.name,
        "column_type": column.column_type,
        "is_nullable": column.is_nullable,
        "column_default": column.column_default,
        "extra": column.extra,
        "character_set_name": column.character_set_name,
        "collation_name": column.collation_name,
        "column_comment": column.column_comment,
    }


def index_to_dict(index: object) -> Dict[str, object]:
    return {
        "name": index.name,
        "non_unique": index.non_unique,
        "index_type": index.index_type,
        "columns": [
            {
                "seq_in_index": column.seq_in_index,
                "column_name": column.column_name,
                "collation": column.collation,
                "sub_part": column.sub_part,
                "nullable": column.nullable,
            }
            for column in index.columns
        ],
    }
