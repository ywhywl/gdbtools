package dbauthlookup

type Options struct {
	BusinessNames       []string
	BusinessClusterFile string
	DBClusterFile       string
	AccessRelationFile  string
	AppIPFile           string
	OutputFormat        string
	OutputPath          string
	AggregateBy         string
	WithDiagnostics     bool
}

type BusinessClusterRow struct {
	Department    string `json:"department,omitempty"`
	BusinessName  string `json:"business_name"`
	Manager       string `json:"manager,omitempty"`
	DBType        string `json:"database_type"`
	ClusterName   string `json:"cluster_name"`
	PrimaryHost   string `json:"primary_host"`
	StandbyHost   string `json:"standby_host,omitempty"`
	TempStandby   string `json:"temp_standby,omitempty"`
	LocalStandby  string `json:"local_standby,omitempty"`
	RemoteStandby string `json:"remote_standby,omitempty"`
}

type DBClusterRow struct {
	ClusterName string `json:"cluster_name"`
	DBNameRaw   string `json:"database_name_raw"`
	DBName      string `json:"database_name"`
	DBType      string `json:"database_type"`
}

type AccessRelationRow struct {
	Seq               string `json:"seq,omitempty"`
	ApplicationName   string `json:"application_name"`
	ApplicationCenter string `json:"application_center"`
	DBNameRaw         string `json:"database_name_raw"`
	DBName            string `json:"database_name"`
	DBPrimaryCenter   string `json:"database_primary_center"`
	DBRole            string `json:"database_role"`
	DBUser            string `json:"db_user"`
	Privilege         string `json:"privilege"`
	Remark            string `json:"remark,omitempty"`
}

type AppIPRow struct {
	ApplicationName   string   `json:"application_name"`
	ApplicationCenter string   `json:"application_center"`
	IPs               []string `json:"ips"`
}

type Dataset struct {
	BusinessClusters []BusinessClusterRow
	DBClusters       []DBClusterRow
	AccessRelations  []AccessRelationRow
	AppIPs           []AppIPRow
	Warnings         []Diagnostic
}

type ResultRow struct {
	Manager           string   `json:"manager,omitempty"`
	BusinessName      string   `json:"business_name"`
	DBType            string   `json:"database_type"`
	ClusterName       string   `json:"cluster_name"`
	PrimaryHost       string   `json:"primary_host"`
	DBName            string   `json:"database_name"`
	ApplicationName   string   `json:"application_name"`
	ApplicationCenter string   `json:"application_center"`
	DBPrimaryCenter   string   `json:"database_primary_center"`
	DBRole            string   `json:"database_role"`
	IPs               []string `json:"ips"`
	DBUser            string   `json:"db_user"`
	Privilege         string   `json:"privilege"`
	Remark            string   `json:"remark,omitempty"`
	MatchStatus       string   `json:"match_status,omitempty"`
	Warning           string   `json:"warning,omitempty"`
}

type Diagnostic struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Source  string `json:"source,omitempty"`
}

type Report struct {
	BusinessNames []string       `json:"business_names,omitempty"`
	AggregateBy   string         `json:"aggregate_by"`
	Summary       Summary        `json:"summary"`
	Console       ConsoleSummary `json:"console_summary"`
	Rows          []ResultRow    `json:"rows"`
	Diagnostics   []Diagnostic   `json:"diagnostics,omitempty"`
}

type Summary struct {
	BusinessClusterRows int `json:"business_cluster_rows"`
	DatabaseCount       int `json:"database_count"`
	ClusterCount        int `json:"cluster_count"`
	ApplicationCount    int `json:"application_count"`
	AuthorizationCount  int `json:"authorization_count"`
	DiagnosticCount     int `json:"diagnostic_count"`
}

type ConsoleSummary struct {
	Total      ConsoleTotal         `json:"total"`
	ByBusiness []BusinessIDCSummary `json:"by_business"`
}

type ConsoleTotal struct {
	BusinessCount      int `json:"business_count"`
	ClusterCount       int `json:"cluster_count"`
	DatabaseCount      int `json:"database_count"`
	AuthorizationCount int `json:"authorization_count"`
	ApplicationCount   int `json:"application_count"`
	IPCount            int `json:"ip_count"`
}

type BusinessIDCSummary struct {
	BusinessName       string `json:"business_name"`
	ApplicationCenter  string `json:"application_center"`
	ClusterCount       int    `json:"cluster_count"`
	DatabaseCount      int    `json:"database_count"`
	AuthorizationCount int    `json:"authorization_count"`
	ApplicationCount   int    `json:"application_count"`
	IPCount            int    `json:"ip_count"`
}
