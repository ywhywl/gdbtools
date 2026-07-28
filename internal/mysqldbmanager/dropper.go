package mysqldbmanager

import "fmt"

func dropDatabase(client *MySQLClient, dbName string) (DropResult, error) {
	result := DropResult{
		Success:  false,
		Database: dbName,
	}

	// Get table count for reporting
	stats, err := client.GetDatabaseStats(dbName)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get database stats: %v", err)
		return result, err
	}
	result.DroppedCount = stats.TableCount

	// Drop the database
	if err := client.DropDatabase(dbName); err != nil {
		result.Error = fmt.Sprintf("Failed to drop database: %v", err)
		return result, err
	}

	result.Success = true
	return result, nil
}
