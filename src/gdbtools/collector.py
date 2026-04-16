from __future__ import annotations

import re
from typing import Dict, List, Sequence, Set, Tuple

from .models import (
    ColumnMeta,
    IndexColumnMeta,
    IndexMeta,
    PrivilegeBundle,
    PrivilegeIdentity,
    SchemaSnapshot,
    TableMeta,
)
from .mysql_client import DatabaseClient
from .utils import (
    SYSTEM_SCHEMAS,
    filter_names,
    matches_selector,
    normalize_whitespace,
    privilege_name_from_column,
    resolve_selectors,
)


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

    excluded = set()
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

    global_rows = client.fetch_rows(
        build_privilege_query("mysql.user", ["User AS user", "Host AS host"], user_columns)
    )
    for row in global_rows:
        user = row["user"]
        host = row["host"]
        if (user, host) not in identities:
            continue
        bundle = ensure_bundle(bundles, user, host, match_mode)
        bundle.hosts.add(host)
        bundle.global_privileges.update(privileges_from_boolean_row(row, user_columns))

    db_rows = client.fetch_rows(
        build_privilege_query("mysql.db", ["User AS user", "Host AS host", "Db AS db_name"], db_columns)
    )
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

    # Preserve users that have no explicit privilege rows after filtering.
    for user, host in identities:
        ensure_bundle(bundles, user, host, match_mode)

    return bundles


def privilege_columns(client: DatabaseClient, table_name: str) -> List[str]:
    rows = client.fetch_rows(f"SHOW COLUMNS FROM {table_name}")
    return [row["Field"] for row in rows if row["Field"].endswith("_priv")]


def build_privilege_query(table_name: str, leading_columns: Sequence[str], privilege_columns_list: Sequence[str]) -> str:
    columns = list(leading_columns) + [f"{column} AS {column}" for column in privilege_columns_list]
    select_clause = ", ".join(columns)
    return f"SELECT {select_clause} FROM {table_name}"


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


def normalize_extra(extra: str | None) -> str:
    if not extra:
        return ""
    normalized = re.sub(r"\bauto_increment\b", "", extra, flags=re.IGNORECASE)
    return normalize_whitespace(normalized) or ""
