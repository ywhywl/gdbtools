# mysql-rename-db Usage

## Overview

`mysql-rename-db` is a Go CLI tool for safely renaming MySQL 5.7 databases using the `RENAME TABLE` approach. It performs comprehensive pre-flight checks to ensure the operation can proceed safely.

## Features

- **Safe database renaming** using MySQL's atomic `RENAME TABLE` operation
- **Comprehensive pre-flight checks**:
  - Cross-database foreign key detection (blocking)
  - Active connection detection (including Sleep state)
  - Recent data modification detection (last 10 days)
  - Table lock detection
  - Replication status detection
  - Permission verification
  - Special objects reporting (views, stored procedures, triggers, events)
- **Flexible credential management**: CLI arguments, JSON config, or MySQL defaults files (`/etc/my.cnf`, `~/.my.cnf`)
- **Dry-run mode** for safe testing
- **Text and JSON output formats**

## Installation

Build from source:

```bash
go build ./cmd/mysql-rename-db
```

Or install directly:

```bash
go install github.com/ywhywl/gdbtools/cmd/mysql-rename-db@latest
```

## Basic Usage

### Rename a database

```bash
mysql-rename-db \
  --host 192.168.1.100 \
  --user root \
  --password secret \
  --old-dbname old_app_db \
  --new-dbname new_app_db
```

### Dry-run mode (recommended first)

```bash
mysql-rename-db \
  --host 192.168.1.100 \
  --user root \
  --password secret \
  --old-dbname old_app_db \
  --new-dbname new_app_db \
  --dry-run
```

### Using defaults file

```bash
# Credentials from /etc/my.cnf [client] section
mysql-rename-db \
  --host 192.168.1.100 \
  --old-dbname old_app_db \
  --new-dbname new_app_db

# Or specify a custom defaults file
mysql-rename-db \
  --defaults-file /path/to/my.cnf \
  --host 192.168.1.100 \
  --old-dbname old_app_db \
  --new-dbname new_app_db
```

### Using JSON config file

Create `config.json`:

```json
{
  "default_user": "admin",
  "default_password": "secret",
  "default_port": 3306
}
```

Run:

```bash
mysql-rename-db \
  --config config.json \
  --host 192.168.1.100 \
  --old-dbname old_app_db \
  --new-dbname new_app_db
```

### Skip pre-checks (NOT recommended)

```bash
mysql-rename-db \
  --host 192.168.1.100 \
  --user root \
  --password secret \
  --old-dbname old_app_db \
  --new-dbname new_app_db \
  --skip-precheck
```

### JSON output

```bash
mysql-rename-db \
  --host 192.168.1.100 \
  --user root \
  --password secret \
  --old-dbname old_app_db \
  --new-dbname new_app_db \
  --output-format json
```

### Save output to file

```bash
mysql-rename-db \
  --host 192.168.1.100 \
  --user root \
  --password secret \
  --old-dbname old_app_db \
  --new-dbname new_app_db \
  --output report.txt
```

## Command-line Options

### Required Options

| Option | Description |
|--------|-------------|
| `--host` | MySQL host IP address (required unless `--socket` is used) |
| `--old-dbname` | Source database name (required) |
| `--new-dbname` | Target database name (required) |

### Authentication Options

Credentials are resolved in the following priority order (highest to lowest):

1. Command-line arguments
2. `--config` JSON file
3. `--defaults-file` specified my.cnf
4. Auto-detected defaults files: `/etc/my.cnf`, `/etc/mysql/my.cnf`, `~/.my.cnf`

| Option | Description |
|--------|-------------|
| `--user` | MySQL username |
| `--password` | MySQL password |
| `--port` | MySQL port (default: 3306) |
| `--socket` | Unix socket path (alternative to `--host`) |
| `--config` | Path to JSON config file |
| `--defaults-file` | Path to MySQL defaults file (e.g., `/etc/my.cnf`) |

### Behavior Options

| Option | Description |
|--------|-------------|
| `--skip-precheck` | Skip all pre-flight checks (NOT recommended) |
| `--dry-run` | Only run checks, don't rename database |
| `--connect-timeout` | Connection timeout in seconds (default: 5) |

### Output Options

| Option | Description |
|--------|-------------|
| `--output-format` | Output format: `text` or `json` (default: `text`) |
| `--output` | Write output to file instead of stdout |

## Pre-flight Checks

### Critical Checks (blocking)

These checks **must pass** or the operation will be blocked:

1. **Source database exists**: Verifies the source database exists
2. **Target database does not exist**: Ensures no naming conflict
3. **Permissions**: Verifies current user has `CREATE`, `DROP`, and `ALTER` privileges
4. **Cross-database foreign keys**: Detects foreign keys that reference other databases or are referenced by other databases (⚠️ **BLOCKS operation**)

### Safety Checks (warnings)

These checks produce warnings but can be bypassed with `--skip-precheck`:

5. **Active connections**: Lists all connections to the source database (including `Sleep` state)
6. **Recent modifications**: Checks for tables modified in the last 10 days using `UPDATE_TIME` (may not be 100% accurate)
7. **Table locks**: Detects locked tables that may cause the operation to hang
8. **Replication status**: Warns if the server is a master (rename will replicate) or slave (should rename on master instead)

### Informational Checks

These checks provide information but don't affect execution:

9. **Special objects**: Reports counts of views, stored procedures, triggers, and events (⚠️ **NOT automatically migrated**)
10. **Database statistics**: Shows table count and total size

## Output Examples

### Text Output (successful)

```
Pre-flight Checks
================================================================

[CRITICAL CHECKS]
✓ Source database exists: old_app_db
✓ Target database does not exist: new_app_db
✓ Current user has sufficient privileges (CREATE, DROP, ALTER)
✓ No cross-database foreign keys

[SAFETY CHECKS]
✓ No active connections to old_app_db
⚠ Data modified in last 10 days:
    - orders (InnoDB, 12500 rows, updated: 2026-07-20T15:30:22Z)
    - products (InnoDB, 3200 rows, updated: 2026-07-18T09:12:45Z)
  Note: Based on UPDATE_TIME, may not be 100% accurate
✓ No table locks detected
✓ No replication configured

[INFORMATIONAL]
⚠ Special objects detected (will NOT be migrated):
    - Views: 5
    - Stored Procedures/Functions: 3
    - Triggers: 2
    - Events: 0
  These objects will NOT be automatically migrated.
  Manual recreation required after rename.
✓ Database has 45 tables, total size: 2345.67 MB

================================================================
Result: PRE-CHECK PASSED WITH WARNINGS
All critical checks passed, but there are warnings.
Please review the warnings before proceeding.

Rename Operation Result
================================================================
✓ Successfully renamed database: old_app_db -> new_app_db
  Renamed 45 table(s)
```

### Text Output (failed - cross-database foreign keys)

```
Pre-flight Checks
================================================================

[CRITICAL CHECKS]
✓ Source database exists: old_app_db
✓ Target database does not exist: new_app_db
✓ Current user has sufficient privileges (CREATE, DROP, ALTER)
✗ Cross-database foreign keys detected (BLOCKING)
  Outbound foreign keys:
    - (source).orders.customer_id -> crm_db.customers.id
  Inbound foreign keys:
    - payment_db.transactions.order_id -> (source).orders.id

================================================================
Result: PRE-CHECK FAILED
One or more critical checks failed. Please resolve the issues before proceeding.

Use --skip-precheck to bypass all checks (NOT recommended).
```

### JSON Output

```json
{
  "pre_check": {
    "passed": true,
    "has_warnings": true,
    "checks": [
      {
        "check_name": "source_database_exists",
        "level": "error",
        "passed": true,
        "message": "Source database exists: old_app_db"
      },
      {
        "check_name": "recent_modifications",
        "level": "warning",
        "passed": false,
        "message": "Data modified in last 10 days",
        "details": {
          "tables": [
            {
              "table_name": "orders",
              "engine": "InnoDB",
              "update_time": "2026-07-20T15:30:22Z",
              "table_rows": 12500
            }
          ],
          "note": "Based on UPDATE_TIME, may not be 100% accurate"
        }
      }
    ]
  },
  "rename_result": {
    "success": true,
    "old_database": "old_app_db",
    "new_database": "new_app_db",
    "renamed_tables": ["users", "orders", "products"]
  },
  "dry_run": false
}
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success - database renamed or dry-run completed |
| `1` | Pre-check failed - operation blocked by critical check failure |
| `2` | Rename operation failed - tables may be partially renamed |
| `3` | Usage error or connection failure |

## How It Works

1. **Connect to MySQL** using provided credentials
2. **Run pre-flight checks** (unless `--skip-precheck`)
   - If any critical check fails, exit with code 1
   - If warnings are present, display them but continue
3. **Exit if dry-run mode** (no changes made)
4. **Execute rename**:
   - Get source database charset and collation
   - Create target database with same charset/collation
   - Rename all tables in a single `RENAME TABLE` statement (atomic within MySQL)
   - Drop empty source database
5. **Output result**

## Important Notes

### What Gets Migrated

✅ **Tables**: All tables are renamed atomically  
✅ **Table data**: No data movement, only metadata changes  
✅ **Table-level character sets**: Preserved on each table  
✅ **Indexes and constraints**: Preserved (except cross-database FKs)

### What Does NOT Get Migrated

❌ **Views**: Contain hardcoded database names, must be recreated manually  
❌ **Stored Procedures/Functions**: Must be recreated in new database  
❌ **Triggers**: Must be recreated in new database  
❌ **Events**: Must be recreated in new database  
❌ **User Privileges**: Database-level grants must be updated manually

### Limitations

- **Cross-database foreign keys** will cause the operation to fail (by design)
- **UPDATE_TIME accuracy**: The "last 10 days modification" check relies on `information_schema.TABLES.UPDATE_TIME`, which may not be accurate for InnoDB tables in some MySQL 5.7 configurations
- **Replication**: If running on a master, the rename will replicate to slaves; if running on a slave, this may cause inconsistency
- **Table locks**: If tables are locked, `RENAME TABLE` will wait, potentially causing the operation to hang

## Best Practices

1. **Always run with `--dry-run` first** to review checks
2. **Execute during low-traffic periods** to minimize risk
3. **Review all warnings** carefully before proceeding
4. **Back up the database** before renaming (just in case)
5. **Check application connection strings** after rename
6. **Manually recreate views/procedures/triggers** after successful rename
7. **Update database-level user privileges** with `GRANT` statements
8. **Verify replication status** if in a master-slave setup

## Troubleshooting

### "Cross-database foreign keys detected"

Remove all foreign key constraints that reference other databases:

```sql
ALTER TABLE old_app_db.orders DROP FOREIGN KEY fk_customer;
```

### "Active connections detected"

Kill the connections before proceeding:

```sql
SHOW PROCESSLIST;
KILL <connection_id>;
```

### "Table locks detected"

Wait for the locks to be released or kill the locking sessions.

### "Data modified in last 10 days"

This is a warning. If you're certain the database is inactive, use `--skip-precheck` or verify with application logs.

### Operation hangs

Check for long-running queries or locks:

```sql
SHOW PROCESSLIST;
SHOW OPEN TABLES WHERE In_use > 0;
```

## See Also

- [mysqlcompare](mysqlcompare_usage.md) - Compare MySQL schema and privileges
- [mysqlpricheck](mysqlpricheck_usage.md) - Audit MySQL privileges
