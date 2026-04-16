# gdbtools

Python 3 MySQL structure and privilege comparison scripts.

## Features

- `mysql_schema_compare.py` compares table structures between a source MySQL and one or more targets
- Structure comparison covers tables, columns, indexes, and selected table options
- `AUTO_INCREMENT` value differences are ignored
- `mysql_privilege_compare.py` compares global, database-level, and table-level privileges
- Privilege comparison supports `user@host` exact matching or merged-by-user matching
- Structure script supports `--source-schemas`, `--target-schemas`, and `--exclude-schemas`
- Privilege script supports `--source-databases`, `--target-databases`, and `--exclude-databases`
- Both scripts accept multiple targets from a single `--target-dsn` using `,`, `|`, or newlines
- Each target result is printed immediately after that target finishes
- A final summary is printed with total, success, failure, consistent, and inconsistent counts
- Shell-friendly summary lines are printed, such as `MYSQL_SCHEMA_COMPARE_FAILED_TARGETS=...`
- Exit code `1` means differences were found, and exit code `2` means at least one target comparison failed
- Structure output shows at most 100 table-difference detail entries per compared database and prints the total difference count

## Quick Start

Choose one of the two scripts:
- `python3 scripts/mysql_schema_compare.py ...`
- `python3 scripts/mysql_privilege_compare.py ...`

## Notes

- The tool prefers `PyMySQL` when installed.
- If `PyMySQL` is not available, it falls back to the local `mysql` client.
- For wildcard matching, selectors use MySQL `LIKE` semantics.
- If a selector exactly matches an existing schema, database, or user, it is treated as an exact match first.
- `--default-user` and `--default-password` apply to `--source-dsn` and every `--target-dsn` entry.
- A single `--target-dsn` value can contain multiple DSNs separated by `,`, `|`, or newlines.
- The structure script is [scripts/mysql_schema_compare.py](/Users/wenlongy/dev/src/gdbtools/scripts/mysql_schema_compare.py).
- The privilege script is [scripts/mysql_privilege_compare.py](/Users/wenlongy/dev/src/gdbtools/scripts/mysql_privilege_compare.py).
- The privilege script imports the structure script as its local shared implementation, so deployment should keep both files under `scripts/` together.
- Exit code `0` means all comparisons succeeded and were consistent, `1` means comparisons completed but differences were found, and `2` means at least one target comparison failed to execute.

## Structure Script

```bash
python3 scripts/mysql_schema_compare.py \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn 'mysql://127.0.0.1:3306/' \
  --target-dsn $'mysql://10.0.0.10:3306/|mysql://10.0.0.11:3306/' \
  --source-schemas 'dbname_0' \
  --target-schemas 'dbname_1,dbname_2' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys'
```

## Privilege Script

```bash
python3 scripts/mysql_privilege_compare.py \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn 'mysql://127.0.0.1:3306/' \
  --target-dsn $'mysql://10.0.0.10:3306/|mysql://10.0.0.11:3306/' \
  --source-databases 'dbname_0' \
  --target-databases 'dbname_1,dbname_2' \
  --exclude-databases 'mysql,information_schema,performance_schema,sys' \
  --exclude-users 'mysql.session,mysql.sys' \
  --user-match-mode user
```
