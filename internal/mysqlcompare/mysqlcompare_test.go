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
	hosts := payload["hosts"].([]string)
	if len(hosts) != 2 {
		t.Fatalf("expected merged hosts, got %#v", payload)
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
	if !strings.Contains(rendered, "table differences: total=101, showing_first=100") {
		t.Fatalf("missing limit header: %s", rendered)
	}
	if !strings.Contains(rendered, "omitted table detail count: 1") {
		t.Fatalf("missing omitted count: %s", rendered)
	}
}

func TestRenderPrivilegeDiffLimitsDetails(t *testing.T) {
	sourceOnly := make([]map[string]any, 0, 101)
	for index := 0; index < 101; index++ {
		sourceOnly = append(sourceOnly, map[string]any{"identity": "app_" + itoa(index)})
	}
	lines := renderPrivilegeDiff(PrivilegeDiff{SourceOnlyIdentities: sourceOnly})
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, "privilege differences: total=101, showing_first=100") {
		t.Fatalf("missing limit header: %s", rendered)
	}
	if !strings.Contains(rendered, "omitted privilege detail count: 1") {
		t.Fatalf("missing omitted count: %s", rendered)
	}
}

func TestRenderTextTargetHonorsCheckMode(t *testing.T) {
	lines := renderTextTarget(TargetComparison{
		Target:            "target_a",
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
