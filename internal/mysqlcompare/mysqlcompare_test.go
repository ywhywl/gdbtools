package mysqlcompare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConnectionDSNWithDefaults(t *testing.T) {
	config, err := parseConnectionDSN("mysql://127.0.0.1:3307/dbname", "", "app", "secret")
	if err != nil {
		t.Fatalf("parseConnectionDSN returned error: %v", err)
	}
	if config.User != "app" || config.Password != "secret" {
		t.Fatalf("unexpected credentials: %#v", config)
	}
	if config.Host != "127.0.0.1" || config.Port != 3307 || config.Database != "dbname" {
		t.Fatalf("unexpected address info: %#v", config)
	}
}

func TestParseTargetDSNsSupportsMultipleSeparators(t *testing.T) {
	targets, err := parseTargetDSNs([]string{"mysql://10.0.0.1:3306/|mysql://10.0.0.2:3306/\nmysql://10.0.0.3:3306/"}, "app", "secret")
	if err != nil {
		t.Fatalf("parseTargetDSNs returned error: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
}

func TestParseSimpleAddressUsesDefaultPort(t *testing.T) {
	config, err := parseConnectionDSN("10.0.0.11", "", "app", "secret")
	if err != nil {
		t.Fatalf("parseConnectionDSN returned error: %v", err)
	}
	if config.Host != "10.0.0.11" || config.Port != 3306 {
		t.Fatalf("unexpected host/port: %#v", config)
	}
}

func TestParseSimpleAddressRespectsCustomPort(t *testing.T) {
	config, err := parseConnectionDSN("10.0.0.11:3307", "", "app", "secret")
	if err != nil {
		t.Fatalf("parseConnectionDSN returned error: %v", err)
	}
	if config.Host != "10.0.0.11" || config.Port != 3307 {
		t.Fatalf("unexpected host/port: %#v", config)
	}
}

func TestMapSchemaPairsSourceExactTargetWildcard(t *testing.T) {
	pairs, err := mapSchemaPairs(
		[]string{"dbname_0"},
		[]string{"dbname_0"},
		[]string{"dbname_0"},
		[]string{"dbname_1", "dbname_2"},
		[]string{"dbname_1", "dbname_2"},
		[]string{"dbname_%"},
	)
	if err != nil {
		t.Fatalf("mapSchemaPairs returned error: %v", err)
	}
	if len(pairs) != 2 || pairs[0].SourceSchema != "dbname_0" || pairs[1].TargetSchema != "dbname_2" {
		t.Fatalf("unexpected pairs: %#v", pairs)
	}
}

func TestEnsureBundleUserModeMergesHosts(t *testing.T) {
	bundles := map[string]*PrivilegeBundle{}
	first := ensureBundle(bundles, "app", "%", "user")
	first.GlobalPrivileges.Add("SELECT")
	first.Hosts.Add("%")

	second := ensureBundle(bundles, "app", "10.%", "user")
	second.GlobalPrivileges.Add("UPDATE")
	second.Hosts.Add("10.%")

	if len(bundles) != 1 {
		t.Fatalf("expected 1 merged bundle, got %d", len(bundles))
	}
	payload := bundles["app"].ToMap()
	if _, found := payload["hosts"]; found {
		t.Fatalf("user mode should not expose hosts in diff payload: %#v", payload)
	}
	if len(bundles["app"].Hosts.Sorted()) != 2 {
		t.Fatalf("expected merged hosts in bundle, got %#v", bundles["app"].Hosts.Sorted())
	}
}

func TestResolveUsersExcludeUserHostIgnoresHostInUserMode(t *testing.T) {
	users, err := resolveUsers(
		fakeDatabaseClient{rowsByQuery: map[string][]Row{
			"\n\t\tSELECT User AS user, Host AS host\n\t\tFROM mysql.user\n\t\tORDER BY User, Host\n\t": {
				{"user": "app", "host": "%"},
				{"user": "app", "host": "10.%"},
				{"user": "other", "host": "%"},
			},
		}},
		nil,
		[]string{"app@%"},
		"user",
	)
	if err != nil {
		t.Fatalf("resolveUsers returned error: %v", err)
	}
	if len(users) != 1 || users[0][0] != "other" {
		t.Fatalf("expected only other user to remain, got %#v", users)
	}
}

func TestDiffPrivilegesUserModeIgnoresHostDifferences(t *testing.T) {
	source := ensureBundle(map[string]*PrivilegeBundle{}, "app", "%", "user")
	source.GlobalPrivileges.Add("SELECT")
	source.Hosts.Add("%")

	target := ensureBundle(map[string]*PrivilegeBundle{}, "app", "10.%", "user")
	target.GlobalPrivileges.Add("SELECT")
	target.Hosts.Add("10.%")

	diff := diffPrivileges(
		map[string]*PrivilegeBundle{"app": source},
		map[string]*PrivilegeBundle{"app": target},
	)
	if diff.HasChanges() {
		t.Fatalf("host-only differences should be ignored in user mode: %#v", diff)
	}
}

func TestRenderSchemaDiffLimitsDetails(t *testing.T) {
	diff := SchemaDiff{
		SourceSchema:     "source_db",
		TargetSchema:     "target_db",
		SourceOnlyTables: makeRangeTables(101),
	}
	lines := renderSchemaDiff(diff)
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, "table differences: total=101, showing_first=20") {
		t.Fatalf("missing limit header: %s", rendered)
	}
	if !strings.Contains(rendered, "omitted table detail count: 81") {
		t.Fatalf("missing omitted count: %s", rendered)
	}
	if !strings.Contains(rendered, "reason: table exists in source but is missing in target") {
		t.Fatalf("missing reason detail: %s", rendered)
	}
}

func TestRenderPrivilegeDiffLimitsDetails(t *testing.T) {
	sourceOnly := make([]map[string]any, 0, 101)
	for index := 0; index < 101; index++ {
		sourceOnly = append(sourceOnly, map[string]any{"identity": "app_" + itoa(index)})
	}
	lines := renderPrivilegeDiff(PrivilegeDiff{SourceOnlyIdentities: sourceOnly})
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, "privilege differences: total=101, showing_first=20") {
		t.Fatalf("missing limit header: %s", rendered)
	}
	if !strings.Contains(rendered, "omitted privilege detail count: 81") {
		t.Fatalf("missing omitted count: %s", rendered)
	}
}

func TestRenderPrivilegeDiffShowsOnlyChangedFields(t *testing.T) {
	lines := renderPrivilegeDiff(PrivilegeDiff{
		ChangedIdentities: []map[string]any{
			{
				"identity": "cmp_priv_b@%",
				"source": map[string]any{
					"identity":          "cmp_priv_b@%",
					"hosts":             []string{"%"},
					"global_privileges": []string{},
					"db_privileges":     map[string]any{},
					"table_privileges": map[string]any{
						"cmp_reason_src.orders": []string{"SELECT"},
					},
				},
				"target": map[string]any{
					"identity":          "cmp_priv_b@%",
					"hosts":             []string{"%"},
					"global_privileges": []string{},
					"db_privileges":     map[string]any{},
					"table_privileges": map[string]any{
						"cmp_reason_src.orders": []string{"UPDATE"},
					},
				},
			},
		},
	})
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, `"table_privileges":{"cmp_reason_src.orders":["SELECT"]}`) {
		t.Fatalf("missing changed privilege field: %s", rendered)
	}
	if strings.Contains(rendered, `"identity":"cmp_priv_b@%"`) || strings.Contains(rendered, `"hosts":["%"]`) {
		t.Fatalf("unchanged privilege fields should not be shown: %s", rendered)
	}
}

func TestRenderTableDiffShowsReasonAndValues(t *testing.T) {
	lines := renderTableDiff(TableDiff{
		Table: "orders",
		ChangedColumns: []map[string]any{
			{
				"column": "status",
				"source": map[string]any{"column_type": "varchar(32)", "is_nullable": false, "ordinal_position": 2},
				"target": map[string]any{"column_type": "varchar(16)", "is_nullable": true, "ordinal_position": 2},
			},
		},
	})
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, "reason: column definition is different between source and target") {
		t.Fatalf("missing column change reason: %s", rendered)
	}
	if !strings.Contains(rendered, `"column_type":"varchar(32)"`) || !strings.Contains(rendered, `"column_type":"varchar(16)"`) {
		t.Fatalf("missing source/target values: %s", rendered)
	}
	if strings.Contains(rendered, `"ordinal_position":2`) {
		t.Fatalf("unchanged fields should not be shown: %s", rendered)
	}
}

func TestDiffSchemaIgnoresIntegerDisplayWidth(t *testing.T) {
	source := SchemaSnapshot{
		Name: "src",
		Tables: map[string]TableMeta{
			"orders": {
				Name: "orders",
				Columns: []ColumnMeta{
					{
						OrdinalPosition: 1,
						Name:            "id",
						ColumnType:      normalizeColumnType("bigint"),
						IsNullable:      false,
						Extra:           normalizeExtra(""),
					},
					{
						OrdinalPosition: 2,
						Name:            "status",
						ColumnType:      normalizeColumnType("int"),
						IsNullable:      false,
						Extra:           normalizeExtra(""),
					},
				},
			},
		},
	}
	target := SchemaSnapshot{
		Name: "tgt",
		Tables: map[string]TableMeta{
			"orders": {
				Name: "orders",
				Columns: []ColumnMeta{
					{
						OrdinalPosition: 1,
						Name:            "id",
						ColumnType:      normalizeColumnType("bigint(20)"),
						IsNullable:      false,
						Extra:           normalizeExtra(""),
					},
					{
						OrdinalPosition: 2,
						Name:            "status",
						ColumnType:      normalizeColumnType("int(11)"),
						IsNullable:      false,
						Extra:           normalizeExtra(""),
					},
				},
			},
		},
	}

	diff := diffSchema(source, target)
	if diff.HasChanges() {
		t.Fatalf("integer display width should be ignored: %#v", diff)
	}
}

func TestDiffSchemaIgnoresDefaultGeneratedInExtra(t *testing.T) {
	source := SchemaSnapshot{
		Name: "src",
		Tables: map[string]TableMeta{
			"orders": {
				Name: "orders",
				Columns: []ColumnMeta{
					{
						OrdinalPosition: 1,
						Name:            "created_at",
						ColumnType:      normalizeColumnType("timestamp"),
						IsNullable:      false,
						Extra:           normalizeExtra("DEFAULT_GENERATED"),
					},
				},
			},
		},
	}
	target := SchemaSnapshot{
		Name: "tgt",
		Tables: map[string]TableMeta{
			"orders": {
				Name: "orders",
				Columns: []ColumnMeta{
					{
						OrdinalPosition: 1,
						Name:            "created_at",
						ColumnType:      normalizeColumnType("timestamp"),
						IsNullable:      false,
						Extra:           normalizeExtra(""),
					},
				},
			},
		},
	}

	diff := diffSchema(source, target)
	if diff.HasChanges() {
		t.Fatalf("default_generated should be ignored in extra: %#v", diff)
	}
}

func TestRenderTextTargetHonorsCheckMode(t *testing.T) {
	lines := renderTextTarget(TargetComparison{
		Target:            "target_a",
		TargetConfig:      ConnectionConfig{Host: "127.0.0.1", Port: 3306},
		PrivilegeDiff:     PrivilegeDiff{},
		IncludeStructure:  false,
		IncludePrivileges: true,
	})
	rendered := strings.Join(lines, "\n")
	if strings.Contains(rendered, "Structure diff") {
		t.Fatalf("structure block should be hidden: %s", rendered)
	}
	if !strings.Contains(rendered, "Privilege diff: no differences") {
		t.Fatalf("privilege block should be shown: %s", rendered)
	}
}

func TestRenderTextSummaryShowsFailedAndInconsistentTargetDetails(t *testing.T) {
	summary := buildSummary([]TargetComparison{
		{
			Target:            "root@10.0.0.11:3306",
			TargetConfig:      ConnectionConfig{Host: "10.0.0.11", Port: 3306},
			SchemaPairs:       []SchemaPair{{SourceSchema: "db_src", TargetSchema: "db_tgt"}},
			PrivilegeDiff:     PrivilegeDiff{},
			IncludeStructure:  true,
			IncludePrivileges: true,
			Error:             "connection failed",
		},
		{
			Target:            "root@10.0.0.12:3307",
			TargetConfig:      ConnectionConfig{Host: "10.0.0.12", Port: 3307},
			SchemaPairs:       []SchemaPair{{SourceSchema: "db_a", TargetSchema: "db_b"}},
			SchemaDiffs:       []SchemaDiff{{SourceSchema: "db_a", TargetSchema: "db_b", SourceOnlyTables: []string{"t1"}}},
			PrivilegeDiff:     PrivilegeDiff{},
			IncludeStructure:  true,
			IncludePrivileges: true,
		},
	})

	rendered := strings.Join(renderTextSummary(summary, 2), "\n")
	if !strings.Contains(rendered, "Failed target details:") {
		t.Fatalf("missing failed target details block: %s", rendered)
	}
	if !strings.Contains(rendered, "host=10.0.0.11") || !strings.Contains(rendered, "port=3306") {
		t.Fatalf("missing failed target host/port: %s", rendered)
	}
	if !strings.Contains(rendered, "compared_schemas=db_a->db_b") {
		t.Fatalf("missing inconsistent target schema info: %s", rendered)
	}
}

func TestParseArgsLoadsDefaultCredentialsFromConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "mysqlcompare.json")
	if err := os.WriteFile(configPath, []byte(`{"default_user":"app","default_password":"secret"}`), 0o600); err != nil {
		t.Fatalf("write config file failed: %v", err)
	}

	options, err := parseArgs([]string{
		"--config", configPath,
		"--source-dsn", "10.0.0.11",
		"--target-dsn", "10.0.0.12:3307",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if options.Source.User != "app" || options.Source.Password != "secret" {
		t.Fatalf("unexpected source credentials: %#v", options.Source)
	}
	if options.Targets[0].Host != "10.0.0.12" || options.Targets[0].Port != 3307 {
		t.Fatalf("unexpected target config: %#v", options.Targets[0])
	}
}

func makeRangeTables(count int) []string {
	items := make([]string, 0, count)
	for index := 0; index < count; index++ {
		items = append(items, "table_"+itoa(index))
	}
	return items
}

type fakeDatabaseClient struct {
	rowsByQuery map[string][]Row
}

func (f fakeDatabaseClient) FetchRows(query string, params ...any) ([]Row, error) {
	return f.rowsByQuery[query], nil
}
