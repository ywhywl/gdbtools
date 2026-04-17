#!/usr/bin/env python3

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from abc import ABC, abstractmethod
from typing import Any, Dict, Iterable, List, Optional, Sequence, Set, Tuple
from urllib.parse import unquote, urlparse

try:
    from dataclasses import asdict, dataclass, field
except ImportError:  # pragma: no cover
    _MISSING = object()

    class _FieldSpec(object):
        def __init__(self, default=_MISSING, default_factory=_MISSING):
            self.default = default
            self.default_factory = default_factory

    def field(default=_MISSING, default_factory=_MISSING):
        if default is not _MISSING and default_factory is not _MISSING:
            raise ValueError("cannot specify both default and default_factory")
        return _FieldSpec(default=default, default_factory=default_factory)

    def dataclass(_cls=None, **kwargs):
        def wrap(cls):
            annotations = getattr(cls, "__annotations__", {})
            field_names = []
            defaults = {}
            factories = {}

            for name in annotations:
                field_names.append(name)
                value = getattr(cls, name, _MISSING)
                if isinstance(value, _FieldSpec):
                    if value.default is not _MISSING:
                        defaults[name] = value.default
                        setattr(cls, name, value.default)
                    elif value.default_factory is not _MISSING:
                        factories[name] = value.default_factory
                        if hasattr(cls, name):
                            delattr(cls, name)
                elif value is not _MISSING:
                    defaults[name] = value

            def __init__(self, *args, **init_kwargs):
                if len(args) > len(field_names):
                    raise TypeError("expected at most %d positional arguments" % len(field_names))
                for index, name in enumerate(field_names):
                    if index < len(args):
                        value = args[index]
                    elif name in init_kwargs:
                        value = init_kwargs.pop(name)
                    elif name in factories:
                        value = factories[name]()
                    elif name in defaults:
                        value = defaults[name]
                    else:
                        raise TypeError("missing required argument: %s" % name)
                    setattr(self, name, value)
                if init_kwargs:
                    unexpected = ", ".join(sorted(init_kwargs))
                    raise TypeError("unexpected keyword arguments: %s" % unexpected)

            cls.__init__ = __init__
            cls.__dataclass_fields__ = tuple(field_names)
            return cls

        if _cls is None:
            return wrap
        return wrap(_cls)

    def asdict(obj):
        if hasattr(obj, "__dataclass_fields__"):
            return {name: asdict(getattr(obj, name)) for name in obj.__dataclass_fields__}
        if isinstance(obj, dict):
            return {key: asdict(value) for key, value in obj.items()}
        if isinstance(obj, (list, tuple)):
            return [asdict(value) for value in obj]
        return obj

try:
    import pymysql
    from pymysql.cursors import DictCursor
except ImportError:  # pragma: no cover
    pymysql = None
    DictCursor = None


SYSTEM_SCHEMAS = {"information_schema", "mysql", "performance_schema", "sys"}
TABLE_OPTION_FIELDS = ["engine", "row_format", "table_collation", "create_options", "table_comment"]
TABLE_DIFF_DETAIL_LIMIT = 100


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
class CompareOptions:
    source: ConnectionConfig
    targets: List[ConnectionConfig]
    source_databases: List[str]
    target_databases: List[str]
    exclude_databases: List[str]
    users: List[str]
    exclude_users: List[str]
    user_match_mode: str
    compare_structure: bool
    compare_privileges: bool
    output_format: str
    mode: str


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

    def table_difference_count(self) -> int:
        return len(self.source_only_tables) + len(self.target_only_tables) + len(self.changed_tables)


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
    include_structure: bool = True
    include_privileges: bool = True
    error: Optional[str] = None

    def is_successful(self) -> bool:
        return self.error is None

    def has_differences(self) -> bool:
        if self.error:
            return False
        return bool(self.schema_diffs) or self.privilege_diff.has_changes()


@dataclass(frozen=True)
class ComparisonSummary:
    total_targets: int
    successful_targets: int
    failed_targets: int
    consistent_targets: int
    inconsistent_targets: int


def split_multi_value(values: Optional[Sequence[str]]) -> List[str]:
    items: List[str] = []
    if not values:
        return items
    for raw_value in values:
        if raw_value is None:
            continue
        for item in re.split(r"[,\|\n]+", raw_value):
            stripped = item.strip()
            if stripped:
                items.append(stripped)
    return items


def normalize_whitespace(value: Optional[str]) -> Optional[str]:
    if value is None:
        return None
    return " ".join(value.split())


def mysql_like_to_regex(pattern: str) -> re.Pattern[str]:
    regex = ["^"]
    escape = False
    for char in pattern:
        if escape:
            regex.append(re.escape(char))
            escape = False
            continue
        if char == "\\":
            escape = True
            continue
        if char == "%":
            regex.append(".*")
        elif char == "_":
            regex.append(".")
        else:
            regex.append(re.escape(char))
    if escape:
        regex.append(re.escape("\\"))
    regex.append("$")
    return re.compile("".join(regex))


def matches_selector(name: str, selector: str) -> bool:
    return bool(mysql_like_to_regex(selector).match(name))


def resolve_selectors(available_names: Sequence[str], selectors: Sequence[str]) -> List[str]:
    selected: List[str] = []
    for selector in selectors:
        if selector in available_names:
            selected.append(selector)
            continue
        for name in available_names:
            if matches_selector(name, selector):
                selected.append(name)
    return sorted(dict.fromkeys(selected))


def filter_names(
    available_names: Iterable[str],
    selectors: Sequence[str],
    exclude_selectors: Sequence[str],
) -> List[str]:
    names = sorted(set(available_names))
    selected = names if not selectors else resolve_selectors(names, selectors)
    if not exclude_selectors:
        return selected
    excluded = set(resolve_selectors(names, exclude_selectors))
    return [name for name in selected if name not in excluded]


def sql_literal(value: object) -> str:
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "1" if value else "0"
    if isinstance(value, (int, float)):
        return str(value)
    text = str(value)
    text = text.replace("\\", "\\\\").replace("'", "\\'")
    return f"'{text}'"


def privilege_name_from_column(column_name: str) -> str:
    stem = column_name[:-5] if column_name.endswith("_priv") else column_name
    return stem.replace("_", " ").upper()


def parse_connection_dsn(
    dsn: str,
    label: Optional[str] = None,
    default_user: Optional[str] = None,
    default_password: Optional[str] = None,
) -> ConnectionConfig:
    parsed = urlparse(dsn)
    if parsed.scheme != "mysql":
        raise ValueError(f"Unsupported DSN scheme: {parsed.scheme or '<empty>'}")
    user = unquote(parsed.username) if parsed.username else default_user
    if not user:
        raise ValueError("DSN must include a username")

    database = parsed.path.lstrip("/")
    socket = None
    host = parsed.hostname or "127.0.0.1"
    port = parsed.port or 3306

    if parsed.hostname in (None, "", "localhost") and parsed.query:
        for query_item in parsed.query.split("&"):
            if not query_item:
                continue
            key, _, value = query_item.partition("=")
            if key == "socket":
                socket = unquote(value)
                host = "localhost"

    return ConnectionConfig(
        dsn=dsn,
        host=host,
        port=port,
        user=user,
        password=unquote(parsed.password) if parsed.password is not None else (default_password or ""),
        database=database,
        socket=socket,
        label=label,
    )


def parse_target_dsns(
    target_dsn_values: List[str],
    default_user: Optional[str] = None,
    default_password: Optional[str] = None,
) -> List[ConnectionConfig]:
    configs: List[ConnectionConfig] = []
    for dsn in split_multi_value(target_dsn_values):
        configs.append(
            parse_connection_dsn(
                dsn,
                default_user=default_user,
                default_password=default_password,
            )
        )
    if not configs:
        raise ValueError("At least one target DSN is required")
    return configs


class DatabaseClient(ABC):
    def __init__(self, config: ConnectionConfig) -> None:
        self.config = config

    @abstractmethod
    def fetch_rows(self, query: str, params: Sequence[Any] = ()) -> List[Dict[str, Any]]:
        raise NotImplementedError


class PyMySQLClient(DatabaseClient):
    def __init__(self, config: ConnectionConfig) -> None:
        super().__init__(config)
        if pymysql is None:
            raise RuntimeError("PyMySQL is not installed")

    def fetch_rows(self, query: str, params: Sequence[Any] = ()) -> List[Dict[str, Any]]:
        connection = pymysql.connect(
            host=self.config.host,
            port=self.config.port,
            user=self.config.user,
            password=self.config.password,
            database=self.config.database or None,
            unix_socket=self.config.socket,
            charset="utf8mb4",
            cursorclass=DictCursor,
        )
        try:
            with connection.cursor() as cursor:
                cursor.execute(query, params)
                return list(cursor.fetchall())
        finally:
            connection.close()


class MySQLCliClient(DatabaseClient):
    def __init__(self, config: ConnectionConfig) -> None:
        super().__init__(config)
        if shutil.which("mysql") is None:
            raise RuntimeError("mysql client is not installed")

    def fetch_rows(self, query: str, params: Sequence[Any] = ()) -> List[Dict[str, Any]]:
        rendered_query = render_query(query, params)
        command = [
            "mysql",
            "--batch",
            "--raw",
            "--default-character-set=utf8mb4",
            "-u",
            self.config.user,
        ]
        if self.config.socket:
            command.extend(["--socket", self.config.socket])
        else:
            command.extend(["-h", self.config.host, "-P", str(self.config.port), "--protocol=tcp"])
        if self.config.database:
            command.append(self.config.database)
        command.extend(["-e", rendered_query])

        env = os.environ.copy()
        if self.config.password:
            env["MYSQL_PWD"] = self.config.password

        result = subprocess.run(
            command,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            message = result.stderr.strip() or result.stdout.strip() or "unknown mysql client error"
            raise RuntimeError(message)
        return parse_mysql_batch_output(result.stdout)


def build_client(config: ConnectionConfig) -> DatabaseClient:
    if pymysql is not None:
        return PyMySQLClient(config)
    return MySQLCliClient(config)


def render_query(query: str, params: Sequence[Any]) -> str:
    rendered = query
    for value in params:
        rendered = rendered.replace("%s", sql_literal(value), 1)
    return rendered


def parse_mysql_batch_output(output: str) -> List[Dict[str, Any]]:
    lines = [line.rstrip("\n") for line in output.splitlines() if line.strip()]
    if not lines:
        return []
    headers = lines[0].split("\t")
    rows: List[Dict[str, Any]] = []
    for line in lines[1:]:
        values = line.split("\t")
        row: Dict[str, Any] = {}
        for index, header in enumerate(headers):
            row[header] = values[index] if index < len(values) else ""
        rows.append(row)
    return rows


def list_schemas(client: DatabaseClient) -> List[str]:
    rows = client.fetch_rows(
        """
        SELECT SCHEMA_NAME AS schema_name
        FROM information_schema.SCHEMATA
        ORDER BY SCHEMA_NAME
        """
    )
    return [row["schema_name"] for row in rows]


def resolve_schemas(
    client: DatabaseClient,
    selectors: Sequence[str],
    exclude_selectors: Sequence[str],
) -> List[str]:
    available = [name for name in list_schemas(client) if name not in SYSTEM_SCHEMAS]
    return filter_names(available, selectors, exclude_selectors)


def collect_schema_snapshot(client: DatabaseClient, schema_name: str) -> SchemaSnapshot:
    table_rows = client.fetch_rows(
        """
        SELECT
            TABLE_NAME AS table_name,
            ENGINE AS engine,
            ROW_FORMAT AS row_format,
            TABLE_COLLATION AS table_collation,
            CREATE_OPTIONS AS create_options,
            TABLE_COMMENT AS table_comment
        FROM information_schema.TABLES
        WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE'
        ORDER BY TABLE_NAME
        """,
        (schema_name,),
    )
    column_rows = client.fetch_rows(
        """
        SELECT
            TABLE_NAME AS table_name,
            ORDINAL_POSITION AS ordinal_position,
            COLUMN_NAME AS column_name,
            COLUMN_TYPE AS column_type,
            IS_NULLABLE AS is_nullable,
            COLUMN_DEFAULT AS column_default,
            EXTRA AS extra,
            CHARACTER_SET_NAME AS character_set_name,
            COLLATION_NAME AS collation_name,
            COLUMN_COMMENT AS column_comment
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = %s
        ORDER BY TABLE_NAME, ORDINAL_POSITION
        """,
        (schema_name,),
    )
    index_rows = client.fetch_rows(
        """
        SELECT
            TABLE_NAME AS table_name,
            INDEX_NAME AS index_name,
            NON_UNIQUE AS non_unique,
            SEQ_IN_INDEX AS seq_in_index,
            COLUMN_NAME AS column_name,
            COLLATION AS collation,
            SUB_PART AS sub_part,
            NULLABLE AS nullable,
            INDEX_TYPE AS index_type
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = %s
        ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX
        """,
        (schema_name,),
    )

    columns_by_table: Dict[str, List[ColumnMeta]] = {}
    for row in column_rows:
        columns_by_table.setdefault(row["table_name"], []).append(
            ColumnMeta(
                ordinal_position=int(row["ordinal_position"]),
                name=row["column_name"],
                column_type=row["column_type"],
                is_nullable=row["is_nullable"] == "YES",
                column_default=row["column_default"],
                extra=normalize_extra(row["extra"]),
                character_set_name=row["character_set_name"] or None,
                collation_name=row["collation_name"] or None,
                column_comment=row["column_comment"] or "",
            )
        )

    raw_indexes: Dict[str, Dict[str, Dict[str, object]]] = {}
    for row in index_rows:
        table_indexes = raw_indexes.setdefault(row["table_name"], {})
        index_payload = table_indexes.setdefault(
            row["index_name"],
            {
                "name": row["index_name"],
                "non_unique": row["non_unique"] in ("1", 1),
                "index_type": row["index_type"],
                "columns": [],
            },
        )
        index_payload["columns"].append(
            IndexColumnMeta(
                seq_in_index=int(row["seq_in_index"]),
                column_name=row["column_name"],
                collation=row["collation"] or None,
                sub_part=int(row["sub_part"]) if row["sub_part"] not in (None, "") else None,
                nullable=(row["nullable"] == "YES") if row["nullable"] not in (None, "") else None,
            )
        )

    indexes_by_table: Dict[str, List[IndexMeta]] = {}
    for table_name, indexes in raw_indexes.items():
        indexes_by_table[table_name] = []
        for _, index_payload in sorted(indexes.items()):
            index_columns = sorted(index_payload["columns"], key=lambda item: item.seq_in_index)
            indexes_by_table[table_name].append(
                IndexMeta(
                    name=index_payload["name"],
                    non_unique=bool(index_payload["non_unique"]),
                    index_type=index_payload["index_type"],
                    columns=tuple(index_columns),
                )
            )

    tables: Dict[str, TableMeta] = {}
    for row in table_rows:
        table_name = row["table_name"]
        tables[table_name] = TableMeta(
            name=table_name,
            engine=row["engine"] or None,
            row_format=row["row_format"] or None,
            table_collation=row["table_collation"] or None,
            create_options=normalize_whitespace(row["create_options"] or None),
            table_comment=row["table_comment"] or None,
            columns=tuple(columns_by_table.get(table_name, [])),
            indexes=tuple(indexes_by_table.get(table_name, [])),
        )
    return SchemaSnapshot(name=schema_name, tables=tables)


def resolve_users(
    client: DatabaseClient,
    selectors: Sequence[str],
    exclude_selectors: Sequence[str],
) -> List[Tuple[str, str]]:
    rows = client.fetch_rows(
        """
        SELECT User AS user, Host AS host
        FROM mysql.user
        ORDER BY User, Host
        """
    )
    identities = [f"{row['user']}@{row['host']}" for row in rows]
    user_to_identities: Dict[str, List[str]] = {}
    for row in rows:
        identity = f"{row['user']}@{row['host']}"
        user_to_identities.setdefault(row["user"], []).append(identity)

    if selectors:
        selected_identities: Set[str] = set()
        for selector in selectors:
            if "@" in selector:
                if selector in identities:
                    selected_identities.add(selector)
                    continue
                selected_identities.update(identity for identity in identities if matches_selector(identity, selector))
                continue

            if selector in user_to_identities:
                selected_identities.update(user_to_identities[selector])
                continue

            for user_name, matched_identities in user_to_identities.items():
                if matches_selector(user_name, selector):
                    selected_identities.update(matched_identities)
    else:
        selected_identities = set(identities)

    excluded: Set[str] = set()
    for selector in exclude_selectors:
        if "@" in selector:
            if selector in identities:
                excluded.add(selector)
            else:
                excluded.update(identity for identity in identities if matches_selector(identity, selector))
        else:
            if selector in user_to_identities:
                excluded.update(user_to_identities[selector])
            else:
                for user_name, matched_identities in user_to_identities.items():
                    if matches_selector(user_name, selector):
                        excluded.update(matched_identities)

    result: List[Tuple[str, str]] = []
    for identity in sorted(selected_identities - excluded):
        user, host = identity.split("@", 1)
        result.append((user, host))
    return result


def collect_privileges(
    client: DatabaseClient,
    users: Sequence[Tuple[str, str]],
    match_mode: str,
    schema_scope: Sequence[str],
) -> Dict[str, PrivilegeBundle]:
    user_columns = privilege_columns(client, "mysql.user")
    db_columns = privilege_columns(client, "mysql.db")
    identities = {(user, host) for user, host in users}
    schema_scope_set = set(schema_scope)

    bundles: Dict[str, PrivilegeBundle] = {}

    global_rows = client.fetch_rows(build_privilege_query("mysql.user", ["User AS user", "Host AS host"], user_columns))
    for row in global_rows:
        user = row["user"]
        host = row["host"]
        if (user, host) not in identities:
            continue
        bundle = ensure_bundle(bundles, user, host, match_mode)
        bundle.hosts.add(host)
        bundle.global_privileges.update(privileges_from_boolean_row(row, user_columns))

    db_rows = client.fetch_rows(build_privilege_query("mysql.db", ["User AS user", "Host AS host", "Db AS db_name"], db_columns))
    for row in db_rows:
        user = row["user"]
        host = row["host"]
        db_name = row["db_name"]
        if (user, host) not in identities:
            continue
        if schema_scope_set and db_name not in schema_scope_set:
            continue
        bundle = ensure_bundle(bundles, user, host, match_mode)
        bundle.hosts.add(host)
        bundle.db_privileges.setdefault(db_name, set()).update(privileges_from_boolean_row(row, db_columns))

    table_rows = client.fetch_rows(
        """
        SELECT
            User AS user,
            Host AS host,
            Db AS db_name,
            Table_name AS table_name,
            Table_priv AS table_priv
        FROM mysql.tables_priv
        ORDER BY User, Host, Db, Table_name
        """
    )
    for row in table_rows:
        user = row["user"]
        host = row["host"]
        db_name = row["db_name"]
        if (user, host) not in identities:
            continue
        if schema_scope_set and db_name not in schema_scope_set:
            continue
        bundle = ensure_bundle(bundles, user, host, match_mode)
        bundle.hosts.add(host)
        bundle.table_privileges.setdefault((db_name, row["table_name"]), set()).update(parse_privilege_set(row["table_priv"]))

    for user, host in identities:
        ensure_bundle(bundles, user, host, match_mode)

    return bundles


def privilege_columns(client: DatabaseClient, table_name: str) -> List[str]:
    rows = client.fetch_rows(f"SHOW COLUMNS FROM {table_name}")
    return [row["Field"] for row in rows if row["Field"].endswith("_priv")]


def build_privilege_query(table_name: str, leading_columns: Sequence[str], privilege_columns_list: Sequence[str]) -> str:
    columns = list(leading_columns) + [f"{column} AS {column}" for column in privilege_columns_list]
    return f"SELECT {', '.join(columns)} FROM {table_name}"


def privileges_from_boolean_row(row: Dict[str, object], columns: Sequence[str]) -> Set[str]:
    privileges: Set[str] = set()
    for column in columns:
        if row.get(column) == "Y":
            privileges.add(privilege_name_from_column(column))
    return privileges


def parse_privilege_set(value: object) -> Set[str]:
    if value in (None, ""):
        return set()
    privileges = set()
    for item in str(value).split(","):
        privilege = item.strip()
        if privilege:
            privileges.add(privilege.replace("_", " ").upper())
    return privileges


def ensure_bundle(
    bundles: Dict[str, PrivilegeBundle],
    user: str,
    host: str,
    match_mode: str,
) -> PrivilegeBundle:
    if match_mode == "user":
        key = user
        identity = PrivilegeIdentity(user=user, host=None)
    else:
        key = f"{user}@{host}"
        identity = PrivilegeIdentity(user=user, host=host)
    bundle = bundles.get(key)
    if bundle is None:
        bundle = PrivilegeBundle(identity=identity)
        bundles[key] = bundle
    return bundle


def remap_privilege_bundles(
    bundles: Dict[str, PrivilegeBundle],
    schema_name_map: Dict[str, str],
) -> Dict[str, PrivilegeBundle]:
    remapped: Dict[str, PrivilegeBundle] = {}
    for key, bundle in bundles.items():
        cloned = PrivilegeBundle(identity=bundle.identity)
        cloned.global_privileges.update(bundle.global_privileges)
        cloned.hosts.update(bundle.hosts)
        for db_name, privileges in bundle.db_privileges.items():
            normalized_name = schema_name_map.get(db_name, db_name)
            cloned.db_privileges.setdefault(normalized_name, set()).update(privileges)
        for (db_name, table_name), privileges in bundle.table_privileges.items():
            normalized_name = schema_name_map.get(db_name, db_name)
            cloned.table_privileges.setdefault((normalized_name, table_name), set()).update(privileges)
        remapped[key] = cloned
    return remapped


def normalize_extra(extra: Optional[str]) -> str:
    if not extra:
        return ""
    normalized = re.sub(r"\bauto_increment\b", "", extra, flags=re.IGNORECASE)
    return normalize_whitespace(normalized) or ""


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

    source_exact = bool(source_selectors) and all(selector in source_available for selector in source_selectors)
    target_exact = bool(target_selectors) and all(selector in target_available for selector in target_selectors)

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
                table_diff.changed_table_options[field_name] = {"source": source_value, "target": target_value}

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
                table_diff.changed_columns.append({"column": column_name, "source": source_column, "target": target_column})

        source_indexes = {index.name: index_to_dict(index) for index in source_table.indexes}
        target_indexes = {index.name: index_to_dict(index) for index in target_table.indexes}
        for index_name in sorted(set(source_indexes) - set(target_indexes)):
            table_diff.source_only_indexes.append(source_indexes[index_name])
        for index_name in sorted(set(target_indexes) - set(source_indexes)):
            table_diff.target_only_indexes.append(target_indexes[index_name])
        for index_name in sorted(set(source_indexes) & set(target_indexes)):
            if source_indexes[index_name] != target_indexes[index_name]:
                table_diff.changed_indexes.append({"index": index_name, "source": source_indexes[index_name], "target": target_indexes[index_name]})

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


def column_to_dict(column: ColumnMeta) -> Dict[str, object]:
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


def index_to_dict(index: IndexMeta) -> Dict[str, object]:
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


def serialize_target_comparison(comparison: TargetComparison) -> Dict[str, object]:
    payload = asdict(comparison)
    payload["status"] = "failed" if comparison.error else "success"
    payload["has_differences"] = comparison.has_differences()
    if comparison.include_structure:
        payload["schema_diffs"] = [serialize_schema_diff(schema_diff) for schema_diff in comparison.schema_diffs]
        payload["schema_pairs"] = [asdict(pair) for pair in comparison.schema_pairs]
    else:
        payload.pop("schema_diffs", None)
        payload.pop("schema_pairs", None)
    if not comparison.include_privileges:
        payload.pop("privilege_diff", None)
    return payload


def serialize_schema_diff(schema_diff: SchemaDiff) -> Dict[str, object]:
    payload = asdict(schema_diff)
    payload["table_difference_count"] = schema_diff.table_difference_count()
    return payload


def render_target_report(comparison: TargetComparison, output_format: str) -> str:
    if output_format == "json":
        return json.dumps(serialize_target_comparison(comparison), ensure_ascii=False, indent=2, sort_keys=True)
    return "\n".join(render_text_target(comparison))


def render_summary_report(summary: ComparisonSummary, output_format: str, exit_code: int) -> str:
    if output_format == "json":
        return json.dumps({"summary": asdict(summary), "exit_code": exit_code}, ensure_ascii=False, indent=2, sort_keys=True)
    return "\n".join(render_text_summary(summary, exit_code))


def render_text_target(comparison: TargetComparison) -> List[str]:
    lines: List[str] = [f"Target: {comparison.target}"]
    if comparison.error:
        lines.append("  Status: failed")
        lines.append(f"  Error: {comparison.error}")
        return lines

    lines.append(f"  Status: {'inconsistent' if comparison.has_differences() else 'consistent'}")
    if comparison.include_structure:
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

    if comparison.include_privileges:
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


def render_text_summary(summary: ComparisonSummary, exit_code: int) -> List[str]:
    return [
        "Summary:",
        f"  Total targets: {summary.total_targets}",
        f"  Successful targets: {summary.successful_targets}",
        f"  Failed targets: {summary.failed_targets}",
        f"  Consistent targets: {summary.consistent_targets}",
        f"  Inconsistent targets: {summary.inconsistent_targets}",
        "ShellSummary:",
        f"MYSQL_SCHEMA_COMPARE_TOTAL_TARGETS={summary.total_targets}",
        f"MYSQL_SCHEMA_COMPARE_SUCCESSFUL_TARGETS={summary.successful_targets}",
        f"MYSQL_SCHEMA_COMPARE_FAILED_TARGETS={summary.failed_targets}",
        f"MYSQL_SCHEMA_COMPARE_CONSISTENT_TARGETS={summary.consistent_targets}",
        f"MYSQL_SCHEMA_COMPARE_INCONSISTENT_TARGETS={summary.inconsistent_targets}",
        f"MYSQL_SCHEMA_COMPARE_EXIT_CODE={exit_code}",
    ]


def render_schema_diff(schema_diff: SchemaDiff) -> List[str]:
    lines: List[str] = [f"    {schema_diff.source_schema} -> {schema_diff.target_schema}"]
    if not schema_diff.has_changes():
        lines.append("      no differences")
        return lines

    total_table_differences = schema_diff.table_difference_count()
    if total_table_differences > TABLE_DIFF_DETAIL_LIMIT:
        lines.append(
            f"      table differences: total={total_table_differences}, showing_first={TABLE_DIFF_DETAIL_LIMIT}"
        )
    else:
        lines.append(f"      table differences: total={total_table_differences}")

    detail_blocks: List[List[str]] = []
    for table_name in schema_diff.source_only_tables:
        detail_blocks.append([f"      source only table: {table_name}"])
    for table_name in schema_diff.target_only_tables:
        detail_blocks.append([f"      target only table: {table_name}"])
    for table_diff in schema_diff.changed_tables:
        detail_blocks.append(render_table_diff(table_diff))

    for block in detail_blocks[:TABLE_DIFF_DETAIL_LIMIT]:
        lines.extend(block)
    omitted_count = total_table_differences - min(total_table_differences, TABLE_DIFF_DETAIL_LIMIT)
    if omitted_count > 0:
        lines.append(f"      omitted table detail count: {omitted_count}")
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


def build_parser(mode: str) -> argparse.ArgumentParser:
    description = "Compare MySQL table structures." if mode == "schema" else "Compare MySQL user privileges."
    parser = argparse.ArgumentParser(description=description)
    parser.add_argument("--source-dsn", required=True, help="Source connection DSN, for example mysql://user:pass@host:3306/")
    parser.add_argument(
        "--target-dsn",
        action="append",
        required=True,
        help="Target connection DSN. Can be repeated or contain comma, pipe, or newline separated DSNs.",
    )
    parser.add_argument("--default-user", help="Default MySQL username used when DSN omits the user part")
    parser.add_argument("--default-password", help="Default MySQL password used when DSN omits the password part")
    if mode == "schema":
        parser.add_argument("--source-schemas", action="append", default=[], help="Source schema selectors")
        parser.add_argument("--target-schemas", action="append", default=[], help="Target schema selectors")
        parser.add_argument("--exclude-schemas", action="append", default=[], help="Schema selectors to exclude")
    else:
        parser.add_argument("--source-databases", action="append", default=[], help="Source database selectors")
        parser.add_argument("--target-databases", action="append", default=[], help="Target database selectors")
        parser.add_argument("--exclude-databases", action="append", default=[], help="Database selectors to exclude")
        parser.add_argument("--users", action="append", default=[], help="User selectors to compare")
        parser.add_argument("--exclude-users", action="append", default=[], help="User selectors to exclude")
        parser.add_argument("--user-match-mode", choices=["user", "user_host"], default="user_host", help="Privilege comparison mode")
    parser.add_argument("--output-format", choices=["text", "json"], default="text", help="Output format")
    return parser


def parse_args(argv: List[str], mode: str) -> CompareOptions:
    parser = build_parser(mode)
    args = parser.parse_args(argv)

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
        source_databases=split_multi_value(getattr(args, "source_schemas", getattr(args, "source_databases", []))),
        target_databases=split_multi_value(getattr(args, "target_schemas", getattr(args, "target_databases", []))),
        exclude_databases=split_multi_value(getattr(args, "exclude_schemas", getattr(args, "exclude_databases", []))),
        users=split_multi_value(getattr(args, "users", [])),
        exclude_users=split_multi_value(getattr(args, "exclude_users", [])),
        user_match_mode=getattr(args, "user_match_mode", "user_host"),
        compare_structure=mode == "schema",
        compare_privileges=mode == "privilege",
        output_format=args.output_format,
        mode=mode,
    )


def build_summary(comparisons: List[TargetComparison]) -> ComparisonSummary:
    total_targets = len(comparisons)
    failed_targets = sum(1 for comparison in comparisons if not comparison.is_successful())
    successful_targets = total_targets - failed_targets
    inconsistent_targets = sum(1 for comparison in comparisons if comparison.has_differences())
    consistent_targets = successful_targets - inconsistent_targets
    return ComparisonSummary(
        total_targets=total_targets,
        successful_targets=successful_targets,
        failed_targets=failed_targets,
        consistent_targets=consistent_targets,
        inconsistent_targets=inconsistent_targets,
    )


def determine_exit_code(summary: ComparisonSummary) -> int:
    if summary.failed_targets > 0:
        return 2
    if summary.inconsistent_targets > 0:
        return 1
    return 0


def main(argv: Optional[List[str]] = None, mode: str = "schema") -> int:
    options = parse_args(argv or sys.argv[1:], mode)
    if options.output_format == "text":
        print(f"Source: {options.source.display_name}", flush=True)

    source_client = build_client(options.source)
    target_clients = [build_client(target) for target in options.targets]

    source_available_schemas = resolve_schemas(source_client, [], options.exclude_databases)
    source_selected_schemas = resolve_schemas(source_client, options.source_databases, options.exclude_databases)
    if not options.source_databases:
        source_selected_schemas = source_available_schemas

    source_schema_cache: Dict[str, SchemaSnapshot] = {}
    source_users = resolve_users(source_client, options.users, options.exclude_users)

    comparisons: List[TargetComparison] = []
    for target, target_client in zip(options.targets, target_clients):
        try:
            target_available_schemas = resolve_schemas(target_client, [], options.exclude_databases)
            target_selected_schemas = resolve_schemas(target_client, options.target_databases, options.exclude_databases)
            if not options.target_databases:
                target_selected_schemas = target_available_schemas

            schema_pairs = map_schema_pairs(
                source_available=source_available_schemas,
                source_selected=source_selected_schemas,
                source_selectors=options.source_databases,
                target_available=target_available_schemas,
                target_selected=target_selected_schemas,
                target_selectors=options.target_databases,
            )

            schema_diffs: List[SchemaDiff] = []
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

            privilege_diff = PrivilegeDiff()
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

            comparison = TargetComparison(
                target=target.display_name,
                schema_pairs=schema_pairs,
                schema_diffs=schema_diffs,
                privilege_diff=privilege_diff,
                include_structure=options.compare_structure,
                include_privileges=options.compare_privileges,
            )
        except Exception as exc:
            comparison = TargetComparison(
                target=target.display_name,
                schema_pairs=[],
                schema_diffs=[],
                privilege_diff=PrivilegeDiff(),
                include_structure=options.compare_structure,
                include_privileges=options.compare_privileges,
                error=str(exc),
            )

        comparisons.append(comparison)
        print(render_target_report(comparison, options.output_format), flush=True)

    summary = build_summary(comparisons)
    exit_code = determine_exit_code(summary)
    print(render_summary_report(summary, options.output_format, exit_code), flush=True)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
