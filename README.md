# gdbtools

Python 3.6+ MySQL structure and privilege comparison scripts.

This repository also includes a Go CLI for MySQL structure and privilege comparison.

## Features

- `mysql_schema_compare.py` compares table structures between a source MySQL and one or more targets
- Structure comparison covers tables, columns, indexes, and selected table options
- `AUTO_INCREMENT` value differences are ignored
- `mysql_privilege_compare.py` compares global, database-level, and table-level privileges
- Privilege comparison supports `user@host` exact matching or merged-by-user matching
- In the Go CLI, global privileges are compared independently from schema mapping, while database-level and table-level privileges are compared only on matched schema pairs
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
- `go install github.com/ywhywl/gdbtools/cmd/mysqlcompare@latest`

## Notes

- The tool prefers `PyMySQL` when installed.
- If `PyMySQL` is not available, it falls back to the local `mysql` client.
- The scripts are intended to run on Python 3.6 and later.
- For wildcard matching, selectors use Linux shell-style glob semantics such as `*`, `?`, and `[]`.
- If a selector exactly matches an existing schema, database, or user, it is treated as an exact match first.
- Old MySQL `LIKE` wildcards such as `%` and `_` are not treated as wildcards by the Go CLI.
- When source and target schema names differ, schema-scoped privilege comparison depends on the schema pairs formed from `--source-schemas` and `--target-schemas`.
- If the Go CLI cannot form any schema pair, it compares only global privileges and ignores database-level and table-level grants.
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

Install remotely:

```bash
go install github.com/ywhywl/gdbtools/cmd/mysqlcompare@latest
```

Download this module into the local Go module cache:

```bash
go get github.com/ywhywl/gdbtools@latest
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
  --target-schemas 'dbname_*' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys' \
  --users 'app_user,report_user@*' \
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

Privilege comparison behavior in the Go CLI:

1. Global privileges such as `GRANT SELECT ON *.*` are compared directly for the same user and do not depend on schema mapping.
2. Database-level privileges such as `GRANT SELECT ON db_name.*` are compared only on matched schema pairs.
3. Table-level privileges such as `GRANT SELECT ON db_name.orders` are compared only on matched schema pairs.
4. If source `db0` is paired with target `db1`, the tool compares source grants on `db0` with target grants on `db1`.
5. Grants on unrelated target schemas are ignored for schema-scoped privilege comparison, even if they reuse the same schema name as the source.
6. If the Go CLI cannot form any schema pair, schema-scoped privileges are skipped and only global privileges are compared.

Example privilege mapping:

- Source selector `--source-schemas 'db0'`
- Target selector `--target-schemas 'db*'`
- If the selected target schema pair is `db0 -> db1`, source database-level privilege for `user1` on `db0` must match target database-level privilege for `user1` on `db1`
- A target-side privilege for `user1` on some other schema such as `db0` does not satisfy the comparison for the `db0 -> db1` pair
