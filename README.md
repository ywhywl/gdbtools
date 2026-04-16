# gdbtools

Python 3 MySQL schema and privilege comparison tool.

## Features

- Compare table structures between a source MySQL and one or more targets
- Compare table metadata, columns, and indexes
- Ignore `AUTO_INCREMENT` value differences
- Compare global, database-level, and table-level privileges
- Support `user@host` exact matching or merged-by-user matching
- Exclude schemas and users from checks
- Support exact schema names and MySQL `LIKE` style selectors
- Accept multiple targets and schema selectors from CLI

## Quick Start

```bash
python3 scripts/mysql_schema_compare.py \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn 'mysql://127.0.0.1:3306/' \
  --target-dsn $'mysql://10.0.0.10:3306/|mysql://10.0.0.11:3306/\nmysql://10.0.0.12:3306/' \
  --source-schemas 'dbname_0' \
  --target-schemas 'dbname_1,dbname_2' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys' \
  --exclude-users 'mysql.session,mysql.sys' \
  --user-match-mode user \
  --output-format text
```

## Notes

- The tool prefers `PyMySQL` when installed.
- If `PyMySQL` is not available, it falls back to the local `mysql` client.
- For wildcard matching, selectors use MySQL `LIKE` semantics.
- If a selector exactly matches an existing schema or user, it is treated as an exact match first.
- `--default-user` and `--default-password` apply to `--source-dsn` and every `--target-dsn` entry.
- A single `--target-dsn` value can contain multiple DSNs separated by `,`, `|`, or newlines.
- The script entrypoint is [scripts/mysql_schema_compare.py](/Users/wenlongy/dev/src/gdbtools/scripts/mysql_schema_compare.py).
