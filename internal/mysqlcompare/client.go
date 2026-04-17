package mysqlcompare

import (
	"database/sql"
	"fmt"

	driver "github.com/go-sql-driver/mysql"
)

type Row map[string]string

type DatabaseClient interface {
	FetchRows(query string, params ...any) ([]Row, error)
}

type MySQLCLIClient struct {
	config ConnectionConfig
	db     *sql.DB
}

func NewMySQLCLIClient(config ConnectionConfig) (*MySQLCLIClient, error) {
	driverConfig := driver.NewConfig()
	driverConfig.User = config.User
	driverConfig.Passwd = config.Password
	driverConfig.DBName = config.Database
	driverConfig.Params = map[string]string{
		"charset": "utf8mb4",
	}
	if config.Socket != "" {
		driverConfig.Net = "unix"
		driverConfig.Addr = config.Socket
	} else {
		driverConfig.Net = "tcp"
		driverConfig.Addr = fmt.Sprintf("%s:%d", config.Host, config.Port)
	}

	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql connection failed: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql failed: %w", err)
	}
	return &MySQLCLIClient{config: config, db: db}, nil
}

func (c *MySQLCLIClient) FetchRows(query string, params ...any) ([]Row, error) {
	rows, err := c.db.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("mysql query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read query columns failed: %w", err)
	}
	result := []Row{}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan query row failed: %w", err)
		}
		row := Row{}
		for index, column := range columns {
			row[column] = databaseValueToString(values[index])
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate query rows failed: %w", err)
	}
	return result, nil
}

func databaseValueToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
