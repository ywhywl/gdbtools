package mysqlrenamedb

import "fmt"

type ConnectionConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Socket   string
	Label    string
}

func (c ConnectionConfig) DisplayName() string {
	if c.Label != "" {
		return c.Label
	}
	if c.Socket != "" {
		return c.User + "@" + c.Socket
	}
	return c.User + "@" + c.Host + ":" + fmt.Sprintf("%d", c.Port)
}

type Options struct {
	Target         ConnectionConfig
	ConfigPath     string
	DefaultsFile   string
	OldDBName      string
	NewDBName      string
	SkipPreCheck   bool
	DryRun         bool
	ConnectTimeout int
	OutputFormat   string
	OutputPath     string
}

type FileConfig struct {
	DefaultUser     string `json:"default_user"`
	DefaultPassword string `json:"default_password"`
	DefaultPort     int    `json:"default_port"`
}

type DefaultsFileConfig struct {
	User     string
	Password string
	Host     string
	Port     int
	Socket   string
}

type PreCheckResult struct {
	CheckName string      `json:"check_name"`
	Level     string      `json:"level"` // "error", "warning", "info"
	Passed    bool        `json:"passed"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
}

type PreCheckReport struct {
	Passed      bool             `json:"passed"`
	HasWarnings bool             `json:"has_warnings"`
	Checks      []PreCheckResult `json:"checks"`
}

type ForeignKeyInfo struct {
	ConstraintName        string `json:"constraint_name"`
	TableName             string `json:"table_name"`
	ColumnName            string `json:"column_name"`
	ReferencedTableSchema string `json:"referenced_table_schema"`
	ReferencedTableName   string `json:"referenced_table_name"`
	ReferencedColumnName  string `json:"referenced_column_name"`
}

type CrossDBForeignKeys struct {
	Outbound []ForeignKeyInfo `json:"outbound,omitempty"`
	Inbound  []ForeignKeyInfo `json:"inbound,omitempty"`
}

type ActiveConnection struct {
	ID      int64   `json:"id"`
	User    string  `json:"user"`
	Host    string  `json:"host"`
	DB      string  `json:"db"`
	Command string  `json:"command"`
	Time    int64   `json:"time"`
	State   *string `json:"state"`
	Info    *string `json:"info"`
}

type ModifiedTable struct {
	TableName  string `json:"table_name"`
	Engine     string `json:"engine"`
	UpdateTime string `json:"update_time"`
	TableRows  int64  `json:"table_rows"`
}

type LockedTable struct {
	TableName string `json:"table_name"`
	InUse     int    `json:"in_use"`
}

type ReplicationStatus struct {
	Role            string `json:"role"` // "master", "slave", "none"
	BinlogFile      string `json:"binlog_file,omitempty"`
	BinlogPosition  int64  `json:"binlog_position,omitempty"`
	MasterHost      string `json:"master_host,omitempty"`
	SlaveIORunning  string `json:"slave_io_running,omitempty"`
	SlaveSQLRunning string `json:"slave_sql_running,omitempty"`
	SecondsBehind   *int64 `json:"seconds_behind_master,omitempty"`
}

type SpecialObjectsCount struct {
	Views    int `json:"views"`
	Routines int `json:"routines"`
	Triggers int `json:"triggers"`
	Events   int `json:"events"`
}

type DatabaseStats struct {
	TableCount int     `json:"table_count"`
	SizeMB     float64 `json:"size_mb"`
}

type RenameResult struct {
	Success       bool     `json:"success"`
	OldDatabase   string   `json:"old_database"`
	NewDatabase   string   `json:"new_database"`
	RenamedTables []string `json:"renamed_tables,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type RunReport struct {
	PreCheck     *PreCheckReport `json:"pre_check,omitempty"`
	RenameResult *RenameResult   `json:"rename_result,omitempty"`
	DryRun       bool            `json:"dry_run"`
}
