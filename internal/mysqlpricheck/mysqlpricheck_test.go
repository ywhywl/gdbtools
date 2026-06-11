package mysqlpricheck

import (
	"strings"
	"testing"
)

type fakeDatabaseClient struct {
	rowsByQuery map[string][]Row
}

func (f fakeDatabaseClient) FetchRows(query string, params ...any) ([]Row, error) {
	return f.rowsByQuery[query], nil
}

func (f fakeDatabaseClient) Close() error {
	return nil
}

func TestParseMyCnfClientSection(t *testing.T) {
	config := parseMyCnf(`
[client]
user=root
password=secret
port=3307
socket=/tmp/mysql.sock

[mysqld]
user=mysql
`)
	if config.User != "root" || config.Password != "secret" {
		t.Fatalf("unexpected credentials: %#v", config)
	}
	if config.Port != 3307 || config.Socket != "/tmp/mysql.sock" {
		t.Fatalf("unexpected port/socket: %#v", config)
	}
}

func TestParseTargetListSupportsMultipleSeparators(t *testing.T) {
	targets, err := parseTargetList([]string{"10.0.0.1:3306|10.0.0.2:3307\n10.0.0.3"}, DefaultsFileConfig{User: "root"}, 3306)
	if err != nil {
		t.Fatalf("parseTargetList returned error: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	if targets[2].Port != 3306 {
		t.Fatalf("expected default port, got %#v", targets[2])
	}
}

func TestResolveUsersSupportsUserAndIdentitySelectors(t *testing.T) {
	users, err := resolveUsers(fakeDatabaseClient{rowsByQuery: map[string][]Row{
		"\n\t\tSELECT User AS user, Host AS host\n\t\tFROM mysql.user\n\t\tORDER BY User, Host\n\t": {
			{"user": "app", "host": "%"},
			{"user": "app", "host": "10.%"},
			{"user": "report", "host": "%"},
		},
	}}, []string{"app@10.%", "report"}, nil, false)
	if err != nil {
		t.Fatalf("resolveUsers returned error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %#v", users)
	}
}

func TestRunRulesFindsInconsistentHosts(t *testing.T) {
	first := newPrivilegeSnapshot(UserHost{User: "app", Host: "%"})
	first.GlobalPrivileges.Add("SELECT")
	second := newPrivilegeSnapshot(UserHost{User: "app", Host: "10.0.0.1"})
	second.GlobalPrivileges.Add("SELECT")
	second.GlobalPrivileges.Add("UPDATE")
	findings := runRules("root@127.0.0.1:3306", []PrivilegeSnapshot{*first, *second}, "host_consistency")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %#v", findings)
	}
	if findings[0].Rule != "inconsistent_host_privileges" {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
}

func TestRunRulesFindsMultiSchemaAndTableLevel(t *testing.T) {
	snapshot := newPrivilegeSnapshot(UserHost{User: "app", Host: "%"})
	snapshot.DBPrivileges["db1"] = StringSet{"SELECT": {}}
	snapshot.DBPrivileges["db2"] = StringSet{"UPDATE": {}}
	snapshot.TablePrivileges[TableScope{Schema: "db1", Table: "orders"}] = StringSet{"SELECT": {}}
	findings := runRules("root@127.0.0.1:3306", []PrivilegeSnapshot{*snapshot}, "all")
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %#v", findings)
	}
}

func TestDetermineExitCodeHonorsFailOn(t *testing.T) {
	reports := []InstanceReport{{
		Findings: []Finding{{Severity: "medium"}},
	}}
	if code := determineExitCode(reports, "high"); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if code := determineExitCode(reports, "medium"); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRenderReportTextIncludesSummary(t *testing.T) {
	output := renderReport(RunReport{
		Reports: []InstanceReport{{
			Instance: "root@127.0.0.1:3306",
			Summary: AuditSummary{
				CheckedUsers:                   1,
				CheckedIdentities:              1,
				MultiSchemaIdentities:          1,
				DBLevelPrivilegeIdentities:     1,
				TableLevelPrivilegeIdentities:  1,
				InconsistentHostPrivilegeUsers: 0,
			},
			Findings: []Finding{{
				Rule:     "multi_schema_privileges",
				Severity: "medium",
				Identity: "app@%",
				Summary:  "app@% has privileges on multiple schemas",
				Details:  map[string]any{"schemas": []string{"db1", "db2"}},
			}},
		}},
		ExitCode: 1,
	}, "text", 20)
	if !strings.Contains(output, "multi_schema_privileges") {
		t.Fatalf("expected finding in output: %s", output)
	}
	if !strings.Contains(output, "checked_users=1") {
		t.Fatalf("expected summary in output: %s", output)
	}
}
