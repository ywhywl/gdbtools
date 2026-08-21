package mysqlpricheck

import (
	"sort"
	"time"
)

type ConnectionConfig struct {
	DSN      string
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
	return c.User + "@" + c.Host + ":" + itoa(c.Port)
}

type Options struct {
	Targets          []ConnectionConfig
	ConfigPath       string
	DefaultsFile     string
	User             string
	Password         string
	Port             int
	Socket           string
	ConnectTimeout   time.Duration
	Users            []string
	ExcludeUsers     []string
	ExcludeSchemas   []string
	IncludeAnonymous bool
	CheckMode        string
	OutputFormat     string
	OutputPath       string
	DetailLimit      int
	FailOn           string
}

type FileConfig struct {
	DefaultUser     string   `json:"default_user"`
	DefaultPassword string   `json:"default_password"`
	DefaultPort     int      `json:"default_port"`
	ExcludeSchemas  []string `json:"exclude_schemas"`
	ExcludeUsers    []string `json:"exclude_users"`
}

type DefaultsFileConfig struct {
	User     string
	Password string
	Host     string
	Port     int
	Socket   string
}

type UserHost struct {
	User string `json:"user"`
	Host string `json:"host"`
}

func (u UserHost) DisplayName() string {
	return u.User + "@" + u.Host
}

type StringSet map[string]struct{}

func (s StringSet) Add(value string) {
	if s == nil || value == "" {
		return
	}
	s[value] = struct{}{}
}

func (s StringSet) Update(values []string) {
	for _, value := range values {
		s.Add(value)
	}
}

func (s StringSet) Sorted() []string {
	items := make([]string, 0, len(s))
	for item := range s {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

type TableScope struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
}

func (s TableScope) DisplayName() string {
	return s.Schema + "." + s.Table
}

type PrivilegeSnapshot struct {
	Identity         UserHost                 `json:"identity"`
	GlobalPrivileges StringSet                `json:"-"`
	DBPrivileges     map[string]StringSet     `json:"-"`
	TablePrivileges  map[TableScope]StringSet `json:"-"`
}

func newPrivilegeSnapshot(identity UserHost) *PrivilegeSnapshot {
	return &PrivilegeSnapshot{
		Identity:         identity,
		GlobalPrivileges: StringSet{},
		DBPrivileges:     map[string]StringSet{},
		TablePrivileges:  map[TableScope]StringSet{},
	}
}

func (s PrivilegeSnapshot) ToMap() map[string]any {
	dbPrivileges := map[string][]string{}
	for schema, privileges := range s.DBPrivileges {
		dbPrivileges[schema] = privileges.Sorted()
	}
	tablePrivileges := map[string][]string{}
	for scope, privileges := range s.TablePrivileges {
		tablePrivileges[scope.DisplayName()] = privileges.Sorted()
	}
	return map[string]any{
		"identity":          s.Identity.DisplayName(),
		"global_privileges": s.GlobalPrivileges.Sorted(),
		"db_privileges":     dbPrivileges,
		"table_privileges":  tablePrivileges,
	}
}

type Finding struct {
	Rule     string         `json:"rule"`
	Severity string         `json:"severity"`
	Instance string         `json:"instance"`
	User     string         `json:"user,omitempty"`
	Identity string         `json:"identity,omitempty"`
	Summary  string         `json:"summary"`
	Details  map[string]any `json:"details"`
}

type AuditSummary struct {
	CheckedUsers                   int `json:"checked_users"`
	CheckedIdentities              int `json:"checked_identities"`
	InconsistentHostPrivilegeUsers int `json:"inconsistent_host_privilege_users"`
	MultiSchemaUsers               int `json:"multi_schema_users"`
	DBLevelPrivilegeUsers          int `json:"db_level_privilege_users"`
	TableLevelPrivilegeUsers       int `json:"table_level_privilege_users"`
	MultiSchemaIdentities          int `json:"multi_schema_identities"`
	DBLevelPrivilegeIdentities     int `json:"db_level_privilege_identities"`
	TableLevelPrivilegeIdentities  int `json:"table_level_privilege_identities"`
	HighSeverityCount              int `json:"high_severity_count"`
	MediumSeverityCount            int `json:"medium_severity_count"`
	LowSeverityCount               int `json:"low_severity_count"`
}

type InstanceReport struct {
	Instance string       `json:"instance"`
	Summary  AuditSummary `json:"summary"`
	Findings []Finding    `json:"findings"`
	Error    string       `json:"error,omitempty"`
}

type RunReport struct {
	Reports   []InstanceReport `json:"reports"`
	ExitCode  int              `json:"exit_code"`
	FailOn    string           `json:"fail_on"`
	Generated string           `json:"generated_at"`
}

type DatabaseClient interface {
	FetchRows(query string, params ...any) ([]Row, error)
	Close() error
}

type Row map[string]string
