package mysqlcompare

import "sort"

const diffDetailLimit = 100

type ConnectionConfig struct {
	DSN      string
	Host     string
	Port     int
	User     string
	Password string
	Database string
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
	Source            ConnectionConfig
	Targets           []ConnectionConfig
	ConfigPath        string
	SourceSchemas     []string
	TargetSchemas     []string
	ExcludeSchemas    []string
	Users             []string
	ExcludeUsers      []string
	UserMatchMode     string
	CompareStructure  bool
	ComparePrivileges bool
	OutputFormat      string
}

type FileConfig struct {
	DefaultUser     string `json:"default_user"`
	DefaultPassword string `json:"default_password"`
}

type ColumnMeta struct {
	OrdinalPosition  int
	Name             string
	ColumnType       string
	IsNullable       bool
	ColumnDefault    *string
	Extra            string
	CharacterSetName *string
	CollationName    *string
	ColumnComment    string
}

type IndexColumnMeta struct {
	SeqInIndex int
	ColumnName string
	Collation  *string
	SubPart    *int
	Nullable   *bool
}

type IndexMeta struct {
	Name      string
	NonUnique bool
	IndexType string
	Columns   []IndexColumnMeta
}

type TableMeta struct {
	Name           string
	Engine         *string
	RowFormat      *string
	TableCollation *string
	CreateOptions  *string
	TableComment   *string
	Columns        []ColumnMeta
	Indexes        []IndexMeta
}

type SchemaSnapshot struct {
	Name   string
	Tables map[string]TableMeta
}

type PrivilegeIdentity struct {
	User string
	Host *string
}

func (p PrivilegeIdentity) DisplayName() string {
	if p.Host == nil {
		return p.User
	}
	return p.User + "@" + *p.Host
}

type StringSet map[string]struct{}

func (s StringSet) Add(value string) {
	if s == nil {
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
	Schema string
	Table  string
}

type PrivilegeBundle struct {
	Identity         PrivilegeIdentity
	GlobalPrivileges StringSet
	DBPrivileges     map[string]StringSet
	TablePrivileges  map[TableScope]StringSet
	Hosts            StringSet
}

func newPrivilegeBundle(identity PrivilegeIdentity) *PrivilegeBundle {
	return &PrivilegeBundle{
		Identity:         identity,
		GlobalPrivileges: StringSet{},
		DBPrivileges:     map[string]StringSet{},
		TablePrivileges:  map[TableScope]StringSet{},
		Hosts:            StringSet{},
	}
}

func (b *PrivilegeBundle) ToMap() map[string]any {
	dbPrivileges := map[string][]string{}
	for dbName, privileges := range b.DBPrivileges {
		dbPrivileges[dbName] = privileges.Sorted()
	}
	tablePrivileges := map[string][]string{}
	for scope, privileges := range b.TablePrivileges {
		tablePrivileges[scope.Schema+"."+scope.Table] = privileges.Sorted()
	}
	return map[string]any{
		"identity":          b.Identity.DisplayName(),
		"hosts":             b.Hosts.Sorted(),
		"global_privileges": b.GlobalPrivileges.Sorted(),
		"db_privileges":     dbPrivileges,
		"table_privileges":  tablePrivileges,
	}
}

type SchemaPair struct {
	SourceSchema string `json:"source_schema"`
	TargetSchema string `json:"target_schema"`
}

type TableDiff struct {
	Table               string
	SourceOnlyColumns   []map[string]any
	TargetOnlyColumns   []map[string]any
	ChangedColumns      []map[string]any
	SourceOnlyIndexes   []map[string]any
	TargetOnlyIndexes   []map[string]any
	ChangedIndexes      []map[string]any
	ChangedTableOptions map[string]map[string]any
}

func (d TableDiff) HasChanges() bool {
	return len(d.SourceOnlyColumns) > 0 ||
		len(d.TargetOnlyColumns) > 0 ||
		len(d.ChangedColumns) > 0 ||
		len(d.SourceOnlyIndexes) > 0 ||
		len(d.TargetOnlyIndexes) > 0 ||
		len(d.ChangedIndexes) > 0 ||
		len(d.ChangedTableOptions) > 0
}

type SchemaDiff struct {
	SourceSchema     string
	TargetSchema     string
	SourceOnlyTables []string
	TargetOnlyTables []string
	ChangedTables    []TableDiff
}

func (d SchemaDiff) HasChanges() bool {
	return len(d.SourceOnlyTables) > 0 || len(d.TargetOnlyTables) > 0 || len(d.ChangedTables) > 0
}

func (d SchemaDiff) TableDifferenceCount() int {
	return len(d.SourceOnlyTables) + len(d.TargetOnlyTables) + len(d.ChangedTables)
}

type PrivilegeDiff struct {
	SourceOnlyIdentities []map[string]any
	TargetOnlyIdentities []map[string]any
	ChangedIdentities    []map[string]any
}

func (d PrivilegeDiff) HasChanges() bool {
	return len(d.SourceOnlyIdentities) > 0 || len(d.TargetOnlyIdentities) > 0 || len(d.ChangedIdentities) > 0
}

func (d PrivilegeDiff) DifferenceCount() int {
	return len(d.SourceOnlyIdentities) + len(d.TargetOnlyIdentities) + len(d.ChangedIdentities)
}

type TargetComparison struct {
	Target            string
	SchemaPairs       []SchemaPair
	SchemaDiffs       []SchemaDiff
	PrivilegeDiff     PrivilegeDiff
	IncludeStructure  bool
	IncludePrivileges bool
	Error             string
}

func (c TargetComparison) IsSuccessful() bool {
	return c.Error == ""
}

func (c TargetComparison) HasDifferences() bool {
	if c.Error != "" {
		return false
	}
	return len(c.SchemaDiffs) > 0 || c.PrivilegeDiff.HasChanges()
}

type ComparisonSummary struct {
	TotalTargets        int
	SuccessfulTargets   int
	FailedTargets       int
	ConsistentTargets   int
	InconsistentTargets int
}
