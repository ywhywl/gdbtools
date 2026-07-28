# mysql-db-manager Usage

## Overview

`mysql-db-manager` is a Go CLI tool for MySQL 5.7 database management with two primary functions:
- **Rename**: Safely rename databases using atomic `RENAME TABLE`
- **Drop**: Safely delete backup databases (names ending with 'bak')

## Installation

```bash
go build ./cmd/mysql-db-manager
# or
go install github.com/ywhywl/gdbtools/cmd/mysql-db-manager@latest
```

## Commands

### rename - Rename Database

Renames a database by creating a new database and moving all tables atomically.

**Basic Usage:**
```bash
mysql-db-manager rename --host <ip> --user <user> --old-dbname <db> [--new-dbname <newdb>]
```

**Features:**
- Default new name: `{old-dbname}bak`
- Comprehensive pre-flight checks (foreign keys, connections, locks, etc.)
- Atomic table renaming (single transaction)
- Preserves charset, collation, and all table structures

**Examples:**
```bash
# Rename with auto-generated name (app_db -> app_dbbak)
mysql-db-manager rename --host 192.168.1.100 --user root --old-dbname app_db

# Rename with custom name
mysql-db-manager rename --host 192.168.1.100 --user root \
  --old-dbname app_db --new-dbname app_db_backup

# Dry-run first (recommended)
mysql-db-manager rename --host 192.168.1.100 --user root \
  --old-dbname app_db --dry-run
```

### drop - Delete Database

Safely deletes backup databases. Requires database name to end with 'bak'.

**Basic Usage:**
```bash
mysql-db-manager drop --host <ip> --user <user> --old-dbname <db_bak>
```

**Features:**
- **Safety check**: Database name must end with 'bak'
- Skips business checks by default (connections, modifications, locks)
- Can enable full checks with `--full-check`
- Does NOT check cross-database foreign keys

**Examples:**
```bash
# Drop backup database (dry-run first)
mysql-db-manager drop --host 192.168.1.100 --user root \
  --old-dbname app_db_bak --dry-run

# Execute drop after reviewing
mysql-db-manager drop --host 192.168.1.100 --user root \
  --old-dbname app_db_bak

# Drop with full pre-flight checks
mysql-db-manager drop --host 192.168.1.100 --user root \
  --old-dbname app_db_bak --full-check
```

## Command-line Options

### Connection Options

| Option | Description | Default |
|--------|-------------|---------|
| `--host` | MySQL host IP | Required (unless --socket) |
| `--port` | MySQL port | 3306 |
| `--user` | MySQL username | From config/defaults file |
| `--password` | MySQL password | From config/defaults file |
| `--socket` | Unix socket path | - |

### Authentication Options

Credentials are resolved in this priority order:
1. Command-line arguments (`--user`, `--password`)
2. `--config` JSON file
3. `--defaults-file` specified my.cnf
4. Auto-detected defaults files: `/etc/my.cnf`, `/etc/mysql/my.cnf`, `~/.my.cnf`

| Option | Description |
|--------|-------------|
| `--config` | Path to JSON config file (format: `{"default_user": "root", "default_password": "secret"}`) |
| `--defaults-file` | Path to MySQL defaults file (reads `[client]` section only) |

**Example my.cnf:**
```ini
[client]
user = root
password = mysecret
port = 3306
```

### Database Options

| Option | Command | Description |
|--------|---------|-------------|
| `--old-dbname` | Both | Source database name (required) |
| `--new-dbname` | rename | Target database name (default: `{old-dbname}bak`) |

### Behavior Options

| Option | Command | Description |
|--------|---------|-------------|
| `--skip-precheck` | Both | Skip ALL pre-checks (dangerous) |
| `--full-check` | drop | Force full pre-flight checks in drop mode |
| `--dry-run` | Both | Show what would happen without making changes |

### Output Options

| Option | Description |
|--------|-------------|
| `--output-format` | Output format: `text` or `json` (default: text) |
| `--output` | Write output to file instead of stdout |
| `--connect-timeout` | Connection timeout in seconds (default: 5) |

## Pre-flight Checks

### Rename Mode

**Critical Checks (blocking):**
- ✓ Source database exists
- ✓ Target database does not exist
- ✓ User has CREATE, DROP, ALTER privileges
- ✓ No cross-database foreign keys

**Safety Checks (warnings):**
- ⚠ Active connections (including Sleep state)
- ⚠ Recent modifications (last 10 days)
- ⚠ Table locks
- ⚠ Replication status (master/slave warnings)

**Informational:**
- ℹ Special objects (views, procedures, triggers, events - NOT migrated)
- ℹ Database statistics (table count, size)

### Drop Mode

**Critical Checks (blocking):**
- ✓ Source database exists
- ✓ Database name ends with 'bak'
- ✓ User has DROP privilege

**Skipped by Default (use `--full-check` to enable):**
- Active connections
- Recent modifications
- Table locks
- Replication status
- Special objects
- Database statistics

**Never Checked in Drop Mode:**
- Cross-database foreign keys

## Output Examples

### Rename Mode - Text Output

```
Pre-flight Checks (Rename Mode)
================================================================

[CRITICAL CHECKS]
✓ Source database exists: app_db
✓ Target database does not exist: app_db_new
✓ Current user has sufficient privileges (CREATE, DROP, ALTER)
✓ No cross-database foreign keys

[SAFETY CHECKS]
✓ No active connections to app_db
⚠ Data modified in last 10 days:
    - orders (InnoDB, 12500 rows, updated: 2026-07-20T15:30:22Z)
  Note: Based on UPDATE_TIME, may not be 100% accurate
✓ No table locks detected
✓ No replication configured

[INFORMATIONAL]
⚠ Special objects detected (will NOT be migrated):
    - Views: 3
    - Stored Procedures/Functions: 2
    - Triggers: 1
    - Events: 0
  These objects will NOT be automatically migrated.
  Manual recreation required after rename.
✓ Database has 25 tables, total size: 512.34 MB

================================================================
Result: PRE-CHECK PASSED WITH WARNINGS
All critical checks passed, but there are warnings.
Please review the warnings before proceeding.

Rename Operation Result
================================================================
✓ Successfully renamed database: app_db -> app_db_new
  Renamed 25 table(s)
```

### Drop Mode - Text Output

```
Pre-flight Checks (Drop Mode)
================================================================

[CRITICAL CHECKS]
✓ Source database exists: app_db_bak
✓ Database name ends with 'bak': app_db_bak
✓ Current user has sufficient privileges (DROP)

[SKIPPED CHECKS]
✓ Active connections check: skipped in drop mode (use --full-check to enable)
✓ Recent modifications check: skipped in drop mode (use --full-check to enable)
✓ Table locks check: skipped in drop mode (use --full-check to enable)
✓ Replication status check: skipped in drop mode (use --full-check to enable)

================================================================
Result: PRE-CHECK PASSED
All checks passed successfully.

Drop Operation Result
================================================================
✓ Successfully dropped database: app_db_bak
  Dropped 25 table(s)
```

### Drop Mode - Name Safety Check Failed

```
Pre-flight Checks (Drop Mode)
================================================================

[CRITICAL CHECKS]
✓ Source database exists: production_db
✗ Database name must end with 'bak' for safe deletion: production_db
✓ Current user has sufficient privileges (DROP)

================================================================
Result: PRE-CHECK FAILED
One or more critical checks failed. Please resolve the issues before proceeding.

Use --skip-precheck to bypass all checks (NOT recommended).
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success - operation completed or dry-run passed |
| 1 | Pre-check failed - critical checks blocked operation |
| 2 | Operation failed - rename or drop failed |
| 3 | Usage error or connection failure |

## What Gets Migrated (Rename)

**✅ Automatically migrated:**
- All tables and their data
- Table structures (columns, indexes, constraints)
- Table-level character sets and collations
- Intra-database foreign keys

**❌ NOT automatically migrated:**
- Views (contain hardcoded database names)
- Stored procedures and functions
- Triggers
- Events
- Database-level user privileges

## Important Notes

### Rename Command

1. **Default naming**: If `--new-dbname` is omitted, new name is `{old-dbname}bak`
2. **Foreign keys**: Cross-database foreign keys block the operation
3. **Views and procedures**: Must be manually recreated after rename
4. **User privileges**: Database-level grants must be manually updated
5. **Replication**: On master servers, rename replicates to all slaves

### Drop Command

1. **Name requirement**: Database name MUST end with 'bak' (safety check)
2. **Bypass with --skip-precheck**: Removes all safety checks including name check
3. **No FK check**: Does NOT check for cross-database foreign keys
4. **Minimal checks**: By default, only checks existence, name, and permissions
5. **Full checks**: Use `--full-check` to run complete pre-flight checks

## Best Practices

### For Rename Operations

1. **Always dry-run first**: `--dry-run` to review checks before executing
2. **Low-traffic periods**: Execute during maintenance windows
3. **Check warnings**: Review all warnings, especially active connections
4. **Backup first**: Take a backup before renaming (just in case)
5. **Update application**: Change connection strings after rename
6. **Recreate objects**: Manually recreate views, procedures, triggers after rename
7. **Update privileges**: Re-grant database-level permissions

### For Drop Operations

1. **Verify backup status**: Confirm the database is truly a backup before dropping
2. **Check dependencies**: Ensure no applications reference this database
3. **Use naming convention**: Always suffix backup databases with 'bak'
4. **Consider full-check**: Use `--full-check` for important backups

## Troubleshooting

### "Cross-database foreign keys detected"

**Rename mode only.** Remove foreign key constraints:
```sql
ALTER TABLE app_db.orders DROP FOREIGN KEY fk_customer;
```

### "Database name must end with 'bak'"

**Drop mode only.** Rename the database first or use `--skip-precheck` (dangerous):
```bash
# Rename first, then drop
mysql-db-manager rename --host ... --old-dbname old_db --new-dbname old_dbbak
mysql-db-manager drop --host ... --old-dbname old_dbbak
```

### "Active connections detected"

Kill connections manually:
```sql
SHOW PROCESSLIST;
KILL <connection_id>;
```

### Operation hangs

Check for locks:
```sql
SHOW PROCESSLIST;
SHOW OPEN TABLES WHERE In_use > 0;
```

## See Also

- [mysqlcompare](mysqlcompare_usage.md) - Compare MySQL schema and privileges
- [mysqlpricheck](mysqlpricheck_usage.md) - Audit MySQL privileges
