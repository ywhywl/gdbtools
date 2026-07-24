package mysqlrenamedb

import (
	"fmt"
	"strings"
)

func runPreChecks(client *MySQLClient, oldDB, newDB string) (PreCheckReport, error) {
	report := PreCheckReport{
		Passed:      true,
		HasWarnings: false,
		Checks:      []PreCheckResult{},
	}

	// Critical checks
	result := checkSourceDatabaseExists(client, oldDB)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.Passed = false
	}

	result = checkTargetDatabaseNotExists(client, newDB)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.Passed = false
	}

	result = checkPermissions(client)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.Passed = false
	}

	result = checkCrossDatabaseForeignKeys(client, oldDB)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.Passed = false
	}

	// Warning-level checks
	result = checkActiveConnections(client, oldDB)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.HasWarnings = true
	}

	result = checkRecentModifications(client, oldDB)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.HasWarnings = true
	}

	result = checkTableLocks(client, oldDB)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.HasWarnings = true
	}

	result = checkReplicationStatus(client)
	report.Checks = append(report.Checks, result)
	if !result.Passed {
		report.HasWarnings = true
	}

	// Info checks
	result = checkSpecialObjects(client, oldDB)
	report.Checks = append(report.Checks, result)

	result = checkDatabaseStats(client, oldDB)
	report.Checks = append(report.Checks, result)

	return report, nil
}

func checkSourceDatabaseExists(client *MySQLClient, dbName string) PreCheckResult {
	exists, err := client.CheckDatabaseExists(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "source_database_exists",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check source database: %v", err),
		}
	}
	if !exists {
		return PreCheckResult{
			CheckName: "source_database_exists",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Source database does not exist: %s", dbName),
		}
	}
	return PreCheckResult{
		CheckName: "source_database_exists",
		Level:     "error",
		Passed:    true,
		Message:   fmt.Sprintf("Source database exists: %s", dbName),
	}
}

func checkTargetDatabaseNotExists(client *MySQLClient, dbName string) PreCheckResult {
	exists, err := client.CheckDatabaseExists(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "target_database_not_exists",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check target database: %v", err),
		}
	}
	if exists {
		return PreCheckResult{
			CheckName: "target_database_not_exists",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Target database already exists: %s", dbName),
		}
	}
	return PreCheckResult{
		CheckName: "target_database_not_exists",
		Level:     "error",
		Passed:    true,
		Message:   fmt.Sprintf("Target database does not exist: %s", dbName),
	}
}

func checkPermissions(client *MySQLClient) PreCheckResult {
	grants, err := client.CheckPermissions()
	if err != nil {
		return PreCheckResult{
			CheckName: "permissions",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check permissions: %v", err),
		}
	}

	hasCreate := false
	hasDrop := false
	hasAlter := false

	grantsText := strings.ToUpper(strings.Join(grants, " "))
	if strings.Contains(grantsText, "ALL PRIVILEGES") || strings.Contains(grantsText, "GRANT ALL") {
		hasCreate = true
		hasDrop = true
		hasAlter = true
	} else {
		hasCreate = strings.Contains(grantsText, "CREATE")
		hasDrop = strings.Contains(grantsText, "DROP")
		hasAlter = strings.Contains(grantsText, "ALTER")
	}

	if !hasCreate || !hasDrop || !hasAlter {
		missing := []string{}
		if !hasCreate {
			missing = append(missing, "CREATE")
		}
		if !hasDrop {
			missing = append(missing, "DROP")
		}
		if !hasAlter {
			missing = append(missing, "ALTER")
		}
		return PreCheckResult{
			CheckName: "permissions",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Missing required privileges: %s", strings.Join(missing, ", ")),
			Details:   grants,
		}
	}

	return PreCheckResult{
		CheckName: "permissions",
		Level:     "error",
		Passed:    true,
		Message:   "Current user has sufficient privileges (CREATE, DROP, ALTER)",
	}
}

func checkCrossDatabaseForeignKeys(client *MySQLClient, dbName string) PreCheckResult {
	fks, err := client.GetCrossDatabaseForeignKeys(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "cross_database_foreign_keys",
			Level:     "error",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check foreign keys: %v", err),
		}
	}

	if len(fks.Outbound) > 0 || len(fks.Inbound) > 0 {
		return PreCheckResult{
			CheckName: "cross_database_foreign_keys",
			Level:     "error",
			Passed:    false,
			Message:   "Cross-database foreign keys detected (BLOCKING)",
			Details:   fks,
		}
	}

	return PreCheckResult{
		CheckName: "cross_database_foreign_keys",
		Level:     "error",
		Passed:    true,
		Message:   "No cross-database foreign keys",
	}
}

func checkActiveConnections(client *MySQLClient, dbName string) PreCheckResult {
	connections, err := client.GetActiveConnections(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "active_connections",
			Level:     "warning",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check connections: %v", err),
		}
	}

	if len(connections) > 0 {
		return PreCheckResult{
			CheckName: "active_connections",
			Level:     "warning",
			Passed:    false,
			Message:   fmt.Sprintf("%d active connection(s) detected (including Sleep)", len(connections)),
			Details:   connections,
		}
	}

	return PreCheckResult{
		CheckName: "active_connections",
		Level:     "warning",
		Passed:    true,
		Message:   fmt.Sprintf("No active connections to %s", dbName),
	}
}

func checkRecentModifications(client *MySQLClient, dbName string) PreCheckResult {
	tables, err := client.GetRecentlyModifiedTables(dbName, 10)
	if err != nil {
		return PreCheckResult{
			CheckName: "recent_modifications",
			Level:     "warning",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check modifications: %v", err),
		}
	}

	if len(tables) > 0 {
		return PreCheckResult{
			CheckName: "recent_modifications",
			Level:     "warning",
			Passed:    false,
			Message:   "Data modified in last 10 days",
			Details: map[string]interface{}{
				"tables": tables,
				"note":   "Based on UPDATE_TIME, may not be 100% accurate",
			},
		}
	}

	return PreCheckResult{
		CheckName: "recent_modifications",
		Level:     "warning",
		Passed:    true,
		Message:   "No recent modifications detected in last 10 days",
	}
}

func checkTableLocks(client *MySQLClient, dbName string) PreCheckResult {
	locks, err := client.GetLockedTables(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "table_locks",
			Level:     "warning",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check table locks: %v", err),
		}
	}

	if len(locks) > 0 {
		return PreCheckResult{
			CheckName: "table_locks",
			Level:     "warning",
			Passed:    false,
			Message:   "Table locks detected",
			Details:   locks,
		}
	}

	return PreCheckResult{
		CheckName: "table_locks",
		Level:     "warning",
		Passed:    true,
		Message:   "No table locks detected",
	}
}

func checkReplicationStatus(client *MySQLClient) PreCheckResult {
	status, err := client.GetReplicationStatus()
	if err != nil {
		return PreCheckResult{
			CheckName: "replication_status",
			Level:     "warning",
			Passed:    false,
			Message:   fmt.Sprintf("Failed to check replication: %v", err),
		}
	}

	if status.Role == "master" {
		return PreCheckResult{
			CheckName: "replication_status",
			Level:     "warning",
			Passed:    false,
			Message:   "This is a MASTER server, rename will be replicated to all slaves",
			Details:   status,
		}
	}

	if status.Role == "slave" {
		return PreCheckResult{
			CheckName: "replication_status",
			Level:     "warning",
			Passed:    false,
			Message:   "This is a SLAVE server, consider performing operation on master instead",
			Details:   status,
		}
	}

	return PreCheckResult{
		CheckName: "replication_status",
		Level:     "info",
		Passed:    true,
		Message:   "No replication configured",
	}
}

func checkSpecialObjects(client *MySQLClient, dbName string) PreCheckResult {
	count, err := client.GetSpecialObjectsCount(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "special_objects",
			Level:     "info",
			Passed:    true,
			Message:   fmt.Sprintf("Failed to check special objects: %v", err),
		}
	}

	total := count.Views + count.Routines + count.Triggers + count.Events
	if total > 0 {
		return PreCheckResult{
			CheckName: "special_objects",
			Level:     "info",
			Passed:    true,
			Message:   "Special objects detected (will NOT be migrated)",
			Details:   count,
		}
	}

	return PreCheckResult{
		CheckName: "special_objects",
		Level:     "info",
		Passed:    true,
		Message:   "No special objects detected",
	}
}

func checkDatabaseStats(client *MySQLClient, dbName string) PreCheckResult {
	stats, err := client.GetDatabaseStats(dbName)
	if err != nil {
		return PreCheckResult{
			CheckName: "database_stats",
			Level:     "info",
			Passed:    true,
			Message:   fmt.Sprintf("Failed to get database stats: %v", err),
		}
	}

	return PreCheckResult{
		CheckName: "database_stats",
		Level:     "info",
		Passed:    true,
		Message:   fmt.Sprintf("Database has %d tables, total size: %.2f MB", stats.TableCount, stats.SizeMB),
		Details:   stats,
	}
}
