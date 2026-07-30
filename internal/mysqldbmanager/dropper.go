package mysqldbmanager

import (
	"fmt"
	"os"
	"time"
)

func dropDatabase(client *MySQLClient, options Options) (DropResult, error) {
	dbName := options.OldDBName
	result := DropResult{
		Success:  false,
		Database: dbName,
	}

	// Get table count and list for reporting
	stats, err := client.GetDatabaseStats(dbName)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get database stats: %v", err)
		return result, err
	}
	result.TotalCount = stats.TableCount

	// Decide drop mode based on batch-size parameter
	if options.BatchSize <= 0 {
		// Direct mode: DROP DATABASE directly
		result.Mode = "direct"
		if err := client.DropDatabase(dbName); err != nil {
			result.Error = fmt.Sprintf("Failed to drop database: %v", err)
			return result, err
		}
		result.DroppedCount = stats.TableCount
	} else {
		// Gradual mode: drop tables in batches
		result.Mode = "gradual"
		droppedTables, err := dropDatabaseGradually(client, dbName, options.BatchSize, options.SleepInterval)
		if err != nil {
			result.Error = fmt.Sprintf("Failed to drop database gradually: %v", err)
			result.DroppedCount = len(droppedTables)
			result.DroppedTables = droppedTables
			return result, err
		}
		result.DroppedCount = len(droppedTables)
		result.DroppedTables = droppedTables
	}

	result.Success = true
	return result, nil
}

func dropDatabaseGradually(client *MySQLClient, dbName string, batchSize int, sleepIntervalMs int) ([]string, error) {
	// Get all tables in the database
	tables, err := client.GetTables(dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %v", err)
	}

	totalTables := len(tables)
	droppedTables := make([]string, 0, totalTables)

	fmt.Fprintf(os.Stderr, "Dropping %d tables from database %s in batches of %d...\n", totalTables, dbName, batchSize)

	// Drop tables in batches
	for i := 0; i < totalTables; i++ {
		tableName := tables[i]

		// Drop single table
		if err := client.DropTable(dbName, tableName); err != nil {
			return droppedTables, fmt.Errorf("failed to drop table %s: %v", tableName, err)
		}
		droppedTables = append(droppedTables, tableName)

		// Progress reporting
		progress := (i + 1) * 100 / totalTables
		fmt.Fprintf(os.Stderr, "Progress: %d%% (%d/%d tables dropped)\n", progress, i+1, totalTables)

		// Sleep between batches to reduce impact
		if (i+1)%batchSize == 0 && i+1 < totalTables {
			fmt.Fprintf(os.Stderr, "Batch complete, sleeping %dms...\n", sleepIntervalMs)
			time.Sleep(time.Duration(sleepIntervalMs) * time.Millisecond)
		}
	}

	fmt.Fprintf(os.Stderr, "All tables dropped, removing empty database...\n")

	// Finally, drop the empty database
	if err := client.DropDatabase(dbName); err != nil {
		return droppedTables, fmt.Errorf("failed to drop empty database: %v", err)
	}

	return droppedTables, nil
}
