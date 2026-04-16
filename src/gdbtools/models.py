from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Dict, List, Optional, Sequence, Set, Tuple


@dataclass(frozen=True)
class ConnectionConfig:
    dsn: str
    host: str
    port: int
    user: str
    password: str
    database: str
    socket: Optional[str] = None
    label: Optional[str] = None

    @property
    def display_name(self) -> str:
        if self.label:
            return self.label
        target = self.socket if self.socket else f"{self.host}:{self.port}"
        return f"{self.user}@{target}"


@dataclass(frozen=True)
class ResolvedPattern:
    selector: str
    matches: Tuple[str, ...]
    exact: bool


@dataclass(frozen=True)
class ColumnMeta:
    ordinal_position: int
    name: str
    column_type: str
    is_nullable: bool
    column_default: Optional[str]
    extra: str
    character_set_name: Optional[str]
    collation_name: Optional[str]
    column_comment: str


@dataclass(frozen=True)
class IndexColumnMeta:
    seq_in_index: int
    column_name: str
    collation: Optional[str]
    sub_part: Optional[int]
    nullable: Optional[bool]


@dataclass(frozen=True)
class IndexMeta:
    name: str
    non_unique: bool
    index_type: str
    columns: Tuple[IndexColumnMeta, ...]


@dataclass(frozen=True)
class TableMeta:
    name: str
    engine: Optional[str]
    row_format: Optional[str]
    table_collation: Optional[str]
    create_options: Optional[str]
    table_comment: Optional[str]
    columns: Tuple[ColumnMeta, ...]
    indexes: Tuple[IndexMeta, ...]


@dataclass(frozen=True)
class SchemaSnapshot:
    name: str
    tables: Dict[str, TableMeta]


@dataclass(frozen=True)
class PrivilegeIdentity:
    user: str
    host: Optional[str]

    @property
    def display_name(self) -> str:
        if self.host is None:
            return self.user
        return f"{self.user}@{self.host}"


@dataclass
class PrivilegeBundle:
    identity: PrivilegeIdentity
    global_privileges: Set[str] = field(default_factory=set)
    db_privileges: Dict[str, Set[str]] = field(default_factory=dict)
    table_privileges: Dict[Tuple[str, str], Set[str]] = field(default_factory=dict)
    hosts: Set[str] = field(default_factory=set)

    def to_dict(self) -> Dict[str, object]:
        return {
            "identity": self.identity.display_name,
            "hosts": sorted(self.hosts),
            "global_privileges": sorted(self.global_privileges),
            "db_privileges": {key: sorted(value) for key, value in sorted(self.db_privileges.items())},
            "table_privileges": {
                f"{schema}.{table}": sorted(value)
                for (schema, table), value in sorted(self.table_privileges.items())
            },
        }


@dataclass(frozen=True)
class SchemaPair:
    source_schema: str
    target_schema: str


@dataclass
class TableDiff:
    table: str
    source_only_columns: List[Dict[str, object]] = field(default_factory=list)
    target_only_columns: List[Dict[str, object]] = field(default_factory=list)
    changed_columns: List[Dict[str, object]] = field(default_factory=list)
    source_only_indexes: List[Dict[str, object]] = field(default_factory=list)
    target_only_indexes: List[Dict[str, object]] = field(default_factory=list)
    changed_indexes: List[Dict[str, object]] = field(default_factory=list)
    changed_table_options: Dict[str, Dict[str, Optional[str]]] = field(default_factory=dict)

    def has_changes(self) -> bool:
        return any(
            [
                self.source_only_columns,
                self.target_only_columns,
                self.changed_columns,
                self.source_only_indexes,
                self.target_only_indexes,
                self.changed_indexes,
                self.changed_table_options,
            ]
        )


@dataclass
class SchemaDiff:
    source_schema: str
    target_schema: str
    source_only_tables: List[str] = field(default_factory=list)
    target_only_tables: List[str] = field(default_factory=list)
    changed_tables: List[TableDiff] = field(default_factory=list)

    def has_changes(self) -> bool:
        return any([self.source_only_tables, self.target_only_tables, self.changed_tables])


@dataclass
class PrivilegeDiff:
    source_only_identities: List[Dict[str, object]] = field(default_factory=list)
    target_only_identities: List[Dict[str, object]] = field(default_factory=list)
    changed_identities: List[Dict[str, object]] = field(default_factory=list)

    def has_changes(self) -> bool:
        return any([self.source_only_identities, self.target_only_identities, self.changed_identities])


@dataclass
class TargetComparison:
    target: str
    schema_pairs: List[SchemaPair]
    schema_diffs: List[SchemaDiff]
    privilege_diff: PrivilegeDiff


@dataclass
class ComparisonReport:
    source: str
    comparisons: List[TargetComparison]

    def to_dict(self) -> Dict[str, object]:
        return {
            "source": self.source,
            "comparisons": [
                {
                    "target": comparison.target,
                    "schema_pairs": [asdict(pair) for pair in comparison.schema_pairs],
                    "schema_diffs": [
                        {
                            "source_schema": diff.source_schema,
                            "target_schema": diff.target_schema,
                            "source_only_tables": diff.source_only_tables,
                            "target_only_tables": diff.target_only_tables,
                            "changed_tables": [asdict(table_diff) for table_diff in diff.changed_tables],
                        }
                        for diff in comparison.schema_diffs
                    ],
                    "privilege_diff": asdict(comparison.privilege_diff),
                }
                for comparison in self.comparisons
            ],
        }
