# gdbtools

Python 3.6+ MySQL structure and privilege comparison scripts.

This repository also includes a Go CLI for MySQL structure and privilege comparison.

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
- `cmd/mysqlcompare` provides a Go implementation based on `database/sql` and `go-sql-driver/mysql`
- The Go CLI supports `--check all|structure|privileges`
- The Go CLI supports simplified connection input such as `10.0.0.11` and `10.0.0.11:3307`
- The Go CLI can read default credentials from a JSON config file

## Quick Start

Choose one of the two scripts:
- `python3 scripts/mysql_schema_compare.py ...`
- `python3 scripts/mysql_privilege_compare.py ...`

Or use the Go CLI:
- `go run ./cmd/mysqlcompare ...`

## Notes

- The tool prefers `PyMySQL` when installed.
- If `PyMySQL` is not available, it falls back to the local `mysql` client.
- The scripts are intended to run on Python 3.6 and later.
- For wildcard matching, selectors use MySQL `LIKE` semantics.
- If a selector exactly matches an existing schema, database, or user, it is treated as an exact match first.
- `--default-user` and `--default-password` apply to `--source-dsn` and every `--target-dsn` entry.
- A single `--target-dsn` value can contain multiple DSNs separated by `,`, `|`, or newlines.
- The structure script is [scripts/mysql_schema_compare.py](/Users/wenlongy/dev/src/gdbtools/scripts/mysql_schema_compare.py).
- The privilege script is [scripts/mysql_privilege_compare.py](/Users/wenlongy/dev/src/gdbtools/scripts/mysql_privilege_compare.py).
- The Go CLI entry is [main.go](/Users/wenlongy/dev/src/gdbtools/cmd/mysqlcompare/main.go).
- The Go implementation is under [internal/mysqlcompare](/Users/wenlongy/dev/src/gdbtools/internal/mysqlcompare).
- The privilege script imports the structure script as its local shared implementation, so deployment should keep both files under `scripts/` together.
- Exit code `0` means all comparisons succeeded and were consistent, `1` means comparisons completed but differences were found, and `2` means at least one target comparison failed to execute.

## Go CLI

Build or run:

```bash
go build ./cmd/mysqlcompare
go run ./cmd/mysqlcompare --help
```

Supported connection input:

- Full DSN such as `mysql://root:password@10.0.0.11:3306/`
- Simplified address such as `10.0.0.11`, which defaults to port `3306`
- Simplified address such as `10.0.0.11:3307`
- Multiple targets in one `--target-dsn` separated by `,`, `|`, or newlines

Credential priority:

1. Username and password in the DSN
2. `--default-user` and `--default-password`
3. `--config` JSON file values

Example config file:

```json
{
  "default_user": "root",
  "default_password": "password"
}
```

Go CLI example:

```bash
go run ./cmd/mysqlcompare \
  --config ./mysqlcompare.json \
  --source-dsn '10.0.0.11' \
  --target-dsn $'10.0.0.12|10.0.0.13:3307' \
  --source-schemas 'dbname_0' \
  --target-schemas 'dbname_%' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys' \
  --users 'app_user,report_user@%' \
  --exclude-users 'mysql.session,mysql.sys' \
  --user-match-mode user \
  --check all \
  --output-format text
```

Go CLI options summary:

- `--source-dsn` source MySQL endpoint, required
- `--target-dsn` target MySQL endpoints, required, repeatable
- `--config` JSON config file with `default_user` and `default_password`
- `--default-user` default username when DSN or simplified address omits it
- `--default-password` default password when DSN or simplified address omits it
- `--source-schemas` or `--source-databases` source schema selectors
- `--target-schemas` or `--target-databases` target schema selectors
- `--exclude-schemas` or `--exclude-databases` excluded schema selectors
- `--users` user selectors, supports `user` and `user@host`
- `--exclude-users` excluded user selectors
- `--user-match-mode` `user` or `user_host`
- `--check` `all`, `structure`, or `privileges`
- `--output-format` `text` or `json`

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
