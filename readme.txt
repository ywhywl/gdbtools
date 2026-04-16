gdbtools

Python 3 MySQL structure and privilege comparison scripts.

Features

- Compare table structures between a source MySQL and one or more targets
- Compare table metadata, columns, and indexes
- Ignore AUTO_INCREMENT value differences
- Compare global, database-level, and table-level privileges
- Support user@host exact matching or merged-by-user matching
- Exclude schemas, databases, and users from checks
- Support exact database names and MySQL LIKE style selectors
- Accept multiple targets and selectors from CLI
- Print each target result immediately after that target finishes
- Print a final summary including success, failure, consistent, and inconsistent counts
- Print shell-friendly summary lines such as MYSQL_SCHEMA_COMPARE_FAILED_TARGETS=...
- Return a non-zero exit code when differences or execution failures are found
- Limit per-schema table difference details to 100 entries and print the total difference count

Quick Start

Choose one of the two scripts:
- python3 scripts/mysql_schema_compare.py ...
- python3 scripts/mysql_privilege_compare.py ...

Notes

- The tool prefers PyMySQL when installed.
- If PyMySQL is not available, it falls back to the local mysql client.
- For wildcard matching, selectors use MySQL LIKE semantics.
- If a selector exactly matches an existing schema, database, or user, it is treated as an exact match first.
- --default-user and --default-password apply to --source-dsn and every --target-dsn entry.
- A single --target-dsn value can contain multiple DSNs separated by comma, pipe, or newlines.
- The structure script is scripts/mysql_schema_compare.py.
- The privilege script is scripts/mysql_privilege_compare.py.
- The two user-facing scripts share one local implementation file for maintenance, but deployment only needs the files under scripts.
- Exit code 0 means all comparisons succeeded and were consistent.
- Exit code 1 means comparisons completed but differences were found.
- Exit code 2 means at least one target comparison failed to execute.

Structure Script

python3 scripts/mysql_schema_compare.py \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn 'mysql://127.0.0.1:3306/' \
  --target-dsn 'mysql://10.0.0.10:3306/|mysql://10.0.0.11:3306/' \
  --source-schemas 'dbname_0' \
  --target-schemas 'dbname_1,dbname_2' \
  --exclude-schemas 'mysql,information_schema,performance_schema,sys'

Privilege Script

python3 scripts/mysql_privilege_compare.py \
  --default-user 'root' \
  --default-password 'password' \
  --source-dsn 'mysql://127.0.0.1:3306/' \
  --target-dsn 'mysql://10.0.0.10:3306/|mysql://10.0.0.11:3306/' \
  --source-databases 'dbname_0' \
  --target-databases 'dbname_1,dbname_2' \
  --exclude-databases 'mysql,information_schema,performance_schema,sys' \
  --exclude-users 'mysql.session,mysql.sys' \
  --user-match-mode user
