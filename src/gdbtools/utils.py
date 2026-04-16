from __future__ import annotations

import re
from typing import Iterable, List, Sequence


SYSTEM_SCHEMAS = {"information_schema", "mysql", "performance_schema", "sys"}


def split_multi_value(values: Sequence[str] | None) -> List[str]:
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


def normalize_whitespace(value: str | None) -> str | None:
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


def selector_is_exact(selector: str, available_names: Sequence[str]) -> bool:
    return selector in available_names


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
