from __future__ import annotations

import os
import shutil
import subprocess
from abc import ABC, abstractmethod
from typing import Any, Dict, List, Sequence

from .models import ConnectionConfig
from .utils import sql_literal

try:
    import pymysql
    from pymysql.cursors import DictCursor
except ImportError:  # pragma: no cover
    pymysql = None
    DictCursor = None


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
