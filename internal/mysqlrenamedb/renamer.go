package mysqlrenamedb

import "fmt"

func renameDatabase(client *MySQLClient, oldDB, newDB string) (RenameResult, error) {
	result := RenameResult{
		Success:     false,
		OldDatabase: oldDB,
		NewDatabase: newDB,
	}

	// Get source database charset and collation
	charset, collation, err := client.GetDatabaseCharset(oldDB)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get source database charset: %v", err)
		return result, err
	}

	// Get all tables from source database
	tables, err := client.GetTables(oldDB)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get tables: %v", err)
		return result, err
	}

	if len(tables) == 0 {
		result.Error = "No tables found in source database"
		return result, fmt.Errorf(result.Error)
	}

	// Create target database
	if err := client.CreateDatabase(newDB, charset, collation); err != nil {
		result.Error = fmt.Sprintf("Failed to create target database: %v", err)
		return result, err
	}

	// Rename all tables in a single statement
	if err := client.RenameTables(oldDB, newDB, tables); err != nil {
		result.Error = fmt.Sprintf("Failed to rename tables: %v", err)
		// Attempt to clean up the empty target database
		_ = client.DropDatabase(newDB)
		return result, err
	}

	result.RenamedTables = tables

	// Drop the old database (should be empty now)
	if err := client.DropDatabase(oldDB); err != nil {
		result.Error = fmt.Sprintf("Tables renamed but failed to drop old database: %v", err)
		result.Success = false
		return result, err
	}

	result.Success = true
	return result, nil
}
