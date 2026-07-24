package mysqlrenamedb

import (
	"encoding/json"
	"fmt"
	"strings"
)

func renderReport(report RunReport, format string) string {
	if format == "json" {
		return renderJSON(report)
	}
	return renderText(report)
}

func renderJSON(report RunReport) string {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal report: %v"}`, err)
	}
	return string(data)
}

func renderText(report RunReport) string {
	var sb strings.Builder

	if report.PreCheck != nil {
		sb.WriteString(renderPreCheckText(*report.PreCheck))
		sb.WriteString("\n")
	}

	if report.RenameResult != nil {
		sb.WriteString(renderRenameResultText(*report.RenameResult))
	}

	if report.DryRun {
		sb.WriteString("\n")
		sb.WriteString("================================================================\n")
		sb.WriteString("DRY RUN MODE: No changes were made\n")
		sb.WriteString("================================================================\n")
	}

	return sb.String()
}

func renderPreCheckText(report PreCheckReport) string {
	var sb strings.Builder

	sb.WriteString("Pre-flight Checks\n")
	sb.WriteString("================================================================\n\n")

	// Group checks by level
	errorChecks := []PreCheckResult{}
	warningChecks := []PreCheckResult{}
	infoChecks := []PreCheckResult{}

	for _, check := range report.Checks {
		switch check.Level {
		case "error":
			errorChecks = append(errorChecks, check)
		case "warning":
			warningChecks = append(warningChecks, check)
		case "info":
			infoChecks = append(infoChecks, check)
		}
	}

	// Render critical checks
	if len(errorChecks) > 0 {
		sb.WriteString("[CRITICAL CHECKS]\n")
		for _, check := range errorChecks {
			sb.WriteString(renderCheckText(check))
		}
		sb.WriteString("\n")
	}

	// Render warning checks
	if len(warningChecks) > 0 {
		sb.WriteString("[SAFETY CHECKS]\n")
		for _, check := range warningChecks {
			sb.WriteString(renderCheckText(check))
		}
		sb.WriteString("\n")
	}

	// Render info checks
	if len(infoChecks) > 0 {
		sb.WriteString("[INFORMATIONAL]\n")
		for _, check := range infoChecks {
			sb.WriteString(renderCheckText(check))
		}
		sb.WriteString("\n")
	}

	// Summary
	sb.WriteString("================================================================\n")
	if !report.Passed {
		sb.WriteString("Result: PRE-CHECK FAILED\n")
		sb.WriteString("One or more critical checks failed. Please resolve the issues before proceeding.\n")
		sb.WriteString("\nUse --skip-precheck to bypass all checks (NOT recommended).\n")
	} else if report.HasWarnings {
		sb.WriteString("Result: PRE-CHECK PASSED WITH WARNINGS\n")
		sb.WriteString("All critical checks passed, but there are warnings.\n")
		sb.WriteString("Please review the warnings before proceeding.\n")
	} else {
		sb.WriteString("Result: PRE-CHECK PASSED\n")
		sb.WriteString("All checks passed successfully.\n")
	}

	return sb.String()
}

func renderCheckText(check PreCheckResult) string {
	var sb strings.Builder

	// Status symbol
	if check.Passed {
		sb.WriteString("✓ ")
	} else {
		if check.Level == "error" {
			sb.WriteString("✗ ")
		} else {
			sb.WriteString("⚠ ")
		}
	}

	// Message
	sb.WriteString(check.Message)
	sb.WriteString("\n")

	// Details
	if check.Details != nil {
		sb.WriteString(renderDetailsText(check.CheckName, check.Details))
	}

	return sb.String()
}

func renderDetailsText(checkName string, details interface{}) string {
	var sb strings.Builder

	switch checkName {
	case "cross_database_foreign_keys":
		if fks, ok := details.(CrossDBForeignKeys); ok {
			if len(fks.Outbound) > 0 {
				sb.WriteString("  Outbound foreign keys:\n")
				for _, fk := range fks.Outbound {
					sb.WriteString(fmt.Sprintf("    - %s.%s.%s -> %s.%s.%s\n",
						"(source)", fk.TableName, fk.ColumnName,
						fk.ReferencedTableSchema, fk.ReferencedTableName, fk.ReferencedColumnName))
				}
			}
			if len(fks.Inbound) > 0 {
				sb.WriteString("  Inbound foreign keys:\n")
				for _, fk := range fks.Inbound {
					sb.WriteString(fmt.Sprintf("    - %s.%s.%s -> (source).%s.%s\n",
						fk.ReferencedTableSchema, fk.TableName, fk.ColumnName,
						fk.ReferencedTableName, fk.ReferencedColumnName))
				}
			}
		}

	case "active_connections":
		if conns, ok := details.([]ActiveConnection); ok {
			for _, conn := range conns {
				state := "NULL"
				if conn.State != nil {
					state = *conn.State
				}
				sb.WriteString(fmt.Sprintf("    - ID: %d, User: %s, Host: %s, Command: %s, Time: %ds, State: %s\n",
					conn.ID, conn.User, conn.Host, conn.Command, conn.Time, state))
			}
		}

	case "recent_modifications":
		if data, ok := details.(map[string]interface{}); ok {
			if tables, ok := data["tables"].([]ModifiedTable); ok {
				for _, table := range tables {
					sb.WriteString(fmt.Sprintf("    - %s (%s, %d rows, updated: %s)\n",
						table.TableName, table.Engine, table.TableRows, table.UpdateTime))
				}
			}
			if note, ok := data["note"].(string); ok {
				sb.WriteString(fmt.Sprintf("  Note: %s\n", note))
			}
		}

	case "table_locks":
		if locks, ok := details.([]LockedTable); ok {
			for _, lock := range locks {
				sb.WriteString(fmt.Sprintf("    - %s (In_use: %d)\n", lock.TableName, lock.InUse))
			}
			sb.WriteString("  Warning: RENAME TABLE will wait for locks to be released.\n")
		}

	case "replication_status":
		if status, ok := details.(ReplicationStatus); ok {
			if status.Role == "master" {
				sb.WriteString(fmt.Sprintf("  Binary log: %s, Position: %d\n", status.BinlogFile, status.BinlogPosition))
				sb.WriteString("  Warning: Rename will be replicated to all slave servers.\n")
			} else if status.Role == "slave" {
				sb.WriteString(fmt.Sprintf("  Master: %s\n", status.MasterHost))
				sb.WriteString(fmt.Sprintf("  Slave_IO_Running: %s, Slave_SQL_Running: %s\n",
					status.SlaveIORunning, status.SlaveSQLRunning))
				if status.SecondsBehind != nil {
					sb.WriteString(fmt.Sprintf("  Seconds_Behind_Master: %d\n", *status.SecondsBehind))
				}
				sb.WriteString("  Warning: Consider performing this operation on the master instead.\n")
			}
		}

	case "special_objects":
		if count, ok := details.(SpecialObjectsCount); ok {
			sb.WriteString(fmt.Sprintf("    - Views: %d\n", count.Views))
			sb.WriteString(fmt.Sprintf("    - Stored Procedures/Functions: %d\n", count.Routines))
			sb.WriteString(fmt.Sprintf("    - Triggers: %d\n", count.Triggers))
			sb.WriteString(fmt.Sprintf("    - Events: %d\n", count.Events))
			sb.WriteString("  These objects will NOT be automatically migrated.\n")
			sb.WriteString("  Manual recreation required after rename.\n")
		}

	case "database_stats":
		// Already included in message
	}

	return sb.String()
}

func renderRenameResultText(result RenameResult) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("Rename Operation Result\n")
	sb.WriteString("================================================================\n")

	if result.Success {
		sb.WriteString(fmt.Sprintf("✓ Successfully renamed database: %s -> %s\n", result.OldDatabase, result.NewDatabase))
		sb.WriteString(fmt.Sprintf("  Renamed %d table(s)\n", len(result.RenamedTables)))
	} else {
		sb.WriteString(fmt.Sprintf("✗ Failed to rename database: %s -> %s\n", result.OldDatabase, result.NewDatabase))
		sb.WriteString(fmt.Sprintf("  Error: %s\n", result.Error))
		if len(result.RenamedTables) > 0 {
			sb.WriteString(fmt.Sprintf("  Partially renamed %d table(s) before failure\n", len(result.RenamedTables)))
		}
	}

	return sb.String()
}
