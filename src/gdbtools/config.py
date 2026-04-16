from __future__ import annotations

from dataclasses import dataclass
from typing import List, Optional
from urllib.parse import unquote, urlparse

from .models import ConnectionConfig
from .utils import split_multi_value


@dataclass(frozen=True)
class CompareOptions:
    source: ConnectionConfig
    targets: List[ConnectionConfig]
    source_schemas: List[str]
    target_schemas: List[str]
    exclude_schemas: List[str]
    users: List[str]
    exclude_users: List[str]
    user_match_mode: str
    compare_structure: bool
    compare_privileges: bool
    output_format: str


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
