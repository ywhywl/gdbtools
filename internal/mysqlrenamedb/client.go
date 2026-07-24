package mysqlrenamedb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLClient struct {
	db *sql.DB
}

func NewMySQLClient(config ConnectionConfig, timeout int) (*MySQLClient, error) {
	dsn := buildDSN(config, timeout)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping server: %w", err)
	}
	return &MySQLClient{db: db}, nil
}

func buildDSN(config ConnectionConfig, timeout int) string {
	if config.Socket != "" {
		return fmt.Sprintf("%s:%s@unix(%s)/?timeout=%ds&parseTime=true",
			config.User, config.Password, config.Socket, timeout)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=%ds&parseTime=true",
		config.User, config.Password, config.Host, config.Port, timeout)
}

func (c *MySQLClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func (c *MySQLClient) CheckDatabaseExists(dbName string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`
	err := c.db.QueryRow(query, dbName).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (c *MySQLClient) GetDatabaseCharset(dbName string) (charset string, collation string, err error) {
	query := `SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME
	          FROM information_schema.SCHEMATA
	          WHERE SCHEMA_NAME = ?`
	err = c.db.QueryRow(query, dbName).Scan(&charset, &collation)
	return
}

func (c *MySQLClient) CheckPermissions() ([]string, error) {
	var grants []string
	rows, err := c.db.Query("SHOW GRANTS FOR CURRENT_USER()")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var grant string
		if err := rows.Scan(&grant); err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (c *MySQLClient) GetCrossDatabaseForeignKeys(dbName string) (CrossDBForeignKeys, error) {
	result := CrossDBForeignKeys{}

	// Check outbound foreign keys (source DB references other DBs)
	outboundQuery := `
		SELECT
			CONSTRAINT_NAME,
			TABLE_NAME,
			COLUMN_NAME,
			REFERENCED_TABLE_SCHEMA,
			REFERENCED_TABLE_NAME,
			REFERENCED_COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ?
		  AND REFERENCED_TABLE_SCHEMA IS NOT NULL
		  AND REFERENCED_TABLE_SCHEMA != ?`

	rows, err := c.db.Query(outboundQuery, dbName, dbName)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var fk ForeignKeyInfo
		if err := rows.Scan(&fk.ConstraintName, &fk.TableName, &fk.ColumnName,
			&fk.ReferencedTableSchema, &fk.ReferencedTableName, &fk.ReferencedColumnName); err != nil {
			return result, err
		}
		result.Outbound = append(result.Outbound, fk)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	// Check inbound foreign keys (other DBs reference source DB)
	inboundQuery := `
		SELECT
			TABLE_SCHEMA,
			TABLE_NAME,
			COLUMN_NAME,
			CONSTRAINT_NAME,
			REFERENCED_TABLE_SCHEMA,
			REFERENCED_TABLE_NAME,
			REFERENCED_COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE REFERENCED_TABLE_SCHEMA = ?
		  AND TABLE_SCHEMA != ?
		  AND REFERENCED_TABLE_SCHEMA IS NOT NULL`

	rows, err = c.db.Query(inboundQuery, dbName, dbName)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var fk ForeignKeyInfo
		var schema string
		if err := rows.Scan(&schema, &fk.TableName, &fk.ColumnName, &fk.ConstraintName,
			&fk.ReferencedTableSchema, &fk.ReferencedTableName, &fk.ReferencedColumnName); err != nil {
			return result, err
		}
		fk.ReferencedTableSchema = schema // Store the referring schema
		result.Inbound = append(result.Inbound, fk)
	}

	return result, rows.Err()
}

func (c *MySQLClient) GetActiveConnections(dbName string) ([]ActiveConnection, error) {
	query := `
		SELECT
			ID,
			USER,
			HOST,
			DB,
			COMMAND,
			TIME,
			STATE,
			INFO
		FROM information_schema.PROCESSLIST
		WHERE DB = ?
		  AND ID != CONNECTION_ID()
		ORDER BY TIME DESC`

	rows, err := c.db.Query(query, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []ActiveConnection
	for rows.Next() {
		var conn ActiveConnection
		if err := rows.Scan(&conn.ID, &conn.User, &conn.Host, &conn.DB,
			&conn.Command, &conn.Time, &conn.State, &conn.Info); err != nil {
			return nil, err
		}
		connections = append(connections, conn)
	}
	return connections, rows.Err()
}

func (c *MySQLClient) GetRecentlyModifiedTables(dbName string, days int) ([]ModifiedTable, error) {
	query := `
		SELECT
			TABLE_NAME,
			ENGINE,
			UPDATE_TIME,
			TABLE_ROWS
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ?
		  AND UPDATE_TIME IS NOT NULL
		  AND UPDATE_TIME > DATE_SUB(NOW(), INTERVAL ? DAY)
		ORDER BY UPDATE_TIME DESC`

	rows, err := c.db.Query(query, dbName, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []ModifiedTable
	for rows.Next() {
		var table ModifiedTable
		var updateTime time.Time
		if err := rows.Scan(&table.TableName, &table.Engine, &updateTime, &table.TableRows); err != nil {
			return nil, err
		}
		table.UpdateTime = updateTime.Format(time.RFC3339)
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func (c *MySQLClient) GetLockedTables(dbName string) ([]LockedTable, error) {
	query := fmt.Sprintf("SHOW OPEN TABLES FROM `%s` WHERE In_use > 0", dbName)
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []LockedTable
	for rows.Next() {
		var table LockedTable
		var database string
		if err := rows.Scan(&database, &table.TableName, &table.InUse); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func (c *MySQLClient) GetReplicationStatus() (ReplicationStatus, error) {
	status := ReplicationStatus{Role: "none"}

	// Check if master
	rows, err := c.db.Query("SHOW MASTER STATUS")
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			status.Role = "master"
			var binlogDoDB, binlogIgnoreDB, executedGtidSet string
			if err := rows.Scan(&status.BinlogFile, &status.BinlogPosition, &binlogDoDB, &binlogIgnoreDB, &executedGtidSet); err == nil {
				return status, nil
			}
		}
	}

	// Check if slave
	rows, err = c.db.Query("SHOW SLAVE STATUS")
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			status.Role = "slave"
			cols, err := rows.Columns()
			if err != nil {
				return status, err
			}
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				return status, err
			}

			// Extract key fields
			for i, col := range cols {
				val := values[i]
				if val == nil {
					continue
				}
				switch col {
				case "Master_Host":
					if v, ok := val.([]byte); ok {
						status.MasterHost = string(v)
					}
				case "Slave_IO_Running":
					if v, ok := val.([]byte); ok {
						status.SlaveIORunning = string(v)
					}
				case "Slave_SQL_Running":
					if v, ok := val.([]byte); ok {
						status.SlaveSQLRunning = string(v)
					}
				case "Seconds_Behind_Master":
					if v, ok := val.(int64); ok {
						status.SecondsBehind = &v
					}
				}
			}
			return status, nil
		}
	}

	return status, nil
}

func (c *MySQLClient) GetSpecialObjectsCount(dbName string) (SpecialObjectsCount, error) {
	count := SpecialObjectsCount{}

	// Count views
	err := c.db.QueryRow(`SELECT COUNT(*) FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ?`, dbName).Scan(&count.Views)
	if err != nil {
		return count, err
	}

	// Count routines (procedures + functions)
	err = c.db.QueryRow(`SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = ?`, dbName).Scan(&count.Routines)
	if err != nil {
		return count, err
	}

	// Count triggers
	err = c.db.QueryRow(`SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ?`, dbName).Scan(&count.Triggers)
	if err != nil {
		return count, err
	}

	// Count events
	err = c.db.QueryRow(`SELECT COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA = ?`, dbName).Scan(&count.Events)
	if err != nil {
		return count, err
	}

	return count, nil
}

func (c *MySQLClient) GetDatabaseStats(dbName string) (DatabaseStats, error) {
	stats := DatabaseStats{}

	// Get table count
	err := c.db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ?`, dbName).Scan(&stats.TableCount)
	if err != nil {
		return stats, err
	}

	// Get total size
	var sizeBytes sql.NullInt64
	err = c.db.QueryRow(`
		SELECT SUM(DATA_LENGTH + INDEX_LENGTH)
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ?`, dbName).Scan(&sizeBytes)
	if err != nil {
		return stats, err
	}
	if sizeBytes.Valid {
		stats.SizeMB = float64(sizeBytes.Int64) / 1024 / 1024
	}

	return stats, nil
}

func (c *MySQLClient) GetTables(dbName string) ([]string, error) {
	query := `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME`
	rows, err := c.db.Query(query, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}
	return tables, rows.Err()
}

func (c *MySQLClient) CreateDatabase(dbName, charset, collation string) error {
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` DEFAULT CHARACTER SET %s COLLATE %s",
		dbName, charset, collation)
	_, err := c.db.Exec(query)
	return err
}

func (c *MySQLClient) RenameTables(oldDB, newDB string, tables []string) error {
	if len(tables) == 0 {
		return nil
	}

	// Build RENAME TABLE statement
	var renamePairs []string
	for _, table := range tables {
		renamePairs = append(renamePairs, fmt.Sprintf("`%s`.`%s` TO `%s`.`%s`", oldDB, table, newDB, table))
	}

	query := "RENAME TABLE " + strings.Join(renamePairs, ", ")
	_, err := c.db.Exec(query)
	return err
}

func (c *MySQLClient) DropDatabase(dbName string) error {
	query := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)
	_, err := c.db.Exec(query)
	return err
}
