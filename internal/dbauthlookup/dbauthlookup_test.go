package dbauthlookup

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExpandDBNames(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "single",
			input: "clearing_branch_00",
			want:  []string{"clearing_branch_00"},
		},
		{
			name:  "full-range-with-zhi",
			input: "clearing_branch_00至clearing_branch_02",
			want:  []string{"clearing_branch_00", "clearing_branch_01", "clearing_branch_02"},
		},
		{
			name:  "full-range-with-dash",
			input: "clearing_branch_00-clearing_branch_02",
			want:  []string{"clearing_branch_00", "clearing_branch_01", "clearing_branch_02"},
		},
		{
			name:  "short-range",
			input: "clearing_branch_00-02",
			want:  []string{"clearing_branch_00", "clearing_branch_01", "clearing_branch_02"},
		},
		{
			name:    "invalid-range",
			input:   "clearing_branch_02-clearing_other_03",
			wantErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandDBNames(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expandDBNames returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expandDBNames mismatch, got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuildReportUsesClusterToDatabaseMappingOnly(t *testing.T) {
	dataset := Dataset{
		BusinessClusters: []BusinessClusterRow{
			{
				BusinessName: "gdb-trans",
				DBType:       "goldendb",
				ClusterName:  "BJ13_clearing_branch_00",
				PrimaryHost:  "10.26.24.11",
			},
		},
		DBClusters: []DBClusterRow{
			{
				ClusterName: "BJ13_clearing_branch_00",
				DBNameRaw:   "clearing_branch_00",
				DBName:      "clearing_branch_00",
				DBType:      "NUDB",
			},
		},
		AccessRelations: []AccessRelationRow{
			{
				ApplicationName:   "clearing-worker",
				ApplicationCenter: "13",
				DBNameRaw:         "clearing_branch_00至clearing_branch_01",
				DBName:            "clearing_branch_00",
				DBPrimaryCenter:   "13",
				DBRole:            "主库",
				DBUser:            "nclearingwork",
				Privilege:         "select,insert,update",
			},
		},
		AppIPs: []AppIPRow{
			{
				ApplicationName:   "clearing-worker",
				ApplicationCenter: "13",
				IPs:               []string{"172.0.10.2"},
			},
		},
	}
	report := buildReport(dataset, Options{BusinessNames: []string{"gdb-trans"}, WithDiagnostics: true})
	if len(report.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(report.Rows))
	}
	if report.Rows[0].DBName != "clearing_branch_00" {
		t.Fatalf("unexpected db name: %s", report.Rows[0].DBName)
	}
	if len(report.Rows[0].IPs) != 1 || report.Rows[0].IPs[0] != "172.0.10.2" {
		t.Fatalf("unexpected ips: %#v", report.Rows[0].IPs)
	}
	if report.Summary.AuthorizationCount != 1 {
		t.Fatalf("unexpected auth count: %d", report.Summary.AuthorizationCount)
	}
}

func TestBuildReportDoesNotFallbackFromClusterName(t *testing.T) {
	dataset := Dataset{
		BusinessClusters: []BusinessClusterRow{
			{
				BusinessName: "gdb-trans",
				DBType:       "goldendb",
				ClusterName:  "BJ13_clearing_branch_00",
				PrimaryHost:  "10.26.24.11",
			},
		},
		AccessRelations: []AccessRelationRow{
			{
				ApplicationName:   "clearing-worker",
				ApplicationCenter: "13",
				DBName:            "clearing_branch_00",
				DBUser:            "nclearingwork",
				Privilege:         "select,insert,update",
			},
		},
	}
	report := buildReport(dataset, Options{BusinessNames: []string{"gdb-trans"}, WithDiagnostics: true})
	if report.Summary.AuthorizationCount != 0 {
		t.Fatalf("expected no rows without cluster mapping, got %d", report.Summary.AuthorizationCount)
	}
	if len(report.Diagnostics) != 1 || report.Diagnostics[0].Type != "missing_cluster_mapping" {
		t.Fatalf("unexpected diagnostics: %#v", report.Diagnostics)
	}
}

func TestBuildReportMatchesAllBusinessesWhenOmitted(t *testing.T) {
	dataset := Dataset{
		BusinessClusters: []BusinessClusterRow{
			{BusinessName: "gdb-trans", DBType: "goldendb", ClusterName: "cluster_a", PrimaryHost: "10.0.0.1"},
			{BusinessName: "gdb-settle", DBType: "goldendb", ClusterName: "cluster_b", PrimaryHost: "10.0.0.2"},
		},
		DBClusters: []DBClusterRow{
			{ClusterName: "cluster_a", DBNameRaw: "db_a", DBName: "db_a", DBType: "NUDB"},
			{ClusterName: "cluster_b", DBNameRaw: "db_b", DBName: "db_b", DBType: "NUDB"},
		},
		AccessRelations: []AccessRelationRow{
			{ApplicationName: "app-a", ApplicationCenter: "13", DBName: "db_a", DBUser: "user_a", Privilege: "select"},
			{ApplicationName: "app-b", ApplicationCenter: "13", DBName: "db_b", DBUser: "user_b", Privilege: "select"},
		},
	}
	report := buildReport(dataset, Options{})
	if report.Summary.AuthorizationCount != 2 {
		t.Fatalf("expected 2 rows, got %d", report.Summary.AuthorizationCount)
	}
}

func TestBuildConsoleSummaryByBusinessIDC(t *testing.T) {
	report := buildReport(Dataset{
		BusinessClusters: []BusinessClusterRow{
			{BusinessName: "gdb-trans", DBType: "goldendb", ClusterName: "cluster_a", PrimaryHost: "10.0.0.1"},
			{BusinessName: "gdb-trans", DBType: "goldendb", ClusterName: "cluster_b", PrimaryHost: "10.0.0.2"},
		},
		DBClusters: []DBClusterRow{
			{ClusterName: "cluster_a", DBNameRaw: "db_a", DBName: "db_a", DBType: "NUDB"},
			{ClusterName: "cluster_b", DBNameRaw: "db_b", DBName: "db_b", DBType: "NUDB"},
		},
		AccessRelations: []AccessRelationRow{
			{ApplicationName: "app-a", ApplicationCenter: "13", DBName: "db_a", DBUser: "user_a", Privilege: "select"},
			{ApplicationName: "app-a", ApplicationCenter: "12", DBName: "db_a", DBUser: "user_a", Privilege: "select"},
			{ApplicationName: "app-a", ApplicationCenter: "13", DBName: "db_b", DBUser: "user_a", Privilege: "select"},
		},
		AppIPs: []AppIPRow{
			{ApplicationName: "app-a", ApplicationCenter: "13", IPs: []string{"172.0.10.1", "172.0.10.2"}},
			{ApplicationName: "app-a", ApplicationCenter: "12", IPs: []string{"172.0.20.1"}},
		},
	}, Options{BusinessNames: []string{"gdb-trans"}})
	output := renderConsoleSummary(report)
	if !strings.Contains(output, "Businesses: 1") {
		t.Fatalf("summary missing business count: %s", output)
	}
	if !strings.Contains(output, "Application IPs: 3") {
		t.Fatalf("summary missing IP count: %s", output)
	}
	if !strings.Contains(output, "- gdb-trans: clusters=2 databases=1 idc-12 applications=1 ips=1 authorization_rows=1") {
		t.Fatalf("summary missing idc-12 row: %s", output)
	}
	if !strings.Contains(output, "- gdb-trans: clusters=2 databases=2 idc-13 applications=1 ips=2 authorization_rows=2") {
		t.Fatalf("summary missing idc-13 row: %s", output)
	}
}

func TestLoadDatasetFromCSVInputs(t *testing.T) {
	dir := t.TempDir()
	businessClusterPath := filepath.Join(dir, "数据库集群映射表.csv")
	dbClusterPath := filepath.Join(dir, "数据库和集群映射表.csv")
	accessRelationPath := filepath.Join(dir, "访问关系表.csv")
	appIPPath := filepath.Join(dir, "应用和ip映射表.csv")
	writeCSVRows(t, businessClusterPath, [][]string{
		{"\ufeff业务名称", "数据库类型", "集群名", "主库"},
		{"gdb-trans", "goldendb", "BJ13_clearing_branch_00", "10.26.24.11"},
	})
	writeCSVRows(t, dbClusterPath, [][]string{
		{"集群名", "数据库名称", "数据库类型"},
		{"BJ13_clearing_branch_00", "clearing_branch_00", "NUDB"},
	})
	writeCSVRows(t, accessRelationPath, [][]string{
		{"序号", "应用名称-CMDB", "应用所属中心", "数据库名称", "数据库主库所属中心", "目标节点数据库角色", "访问数据库使用用户", "访问权限"},
		{"1", "clearing-worker", "13", "clearing_branch_00", "13", "主库", "nclearingwork", "select,insert,update"},
	})
	writeCSVRows(t, appIPPath, [][]string{
		{"应用名称-CMDB", "应用所属中心", "IP"},
		{"clearing-worker", "13", "172.0.10.2,172.0.10.3"},
	})
	dataset, err := loadDataset(Options{
		BusinessClusterFile: businessClusterPath,
		DBClusterFile:       dbClusterPath,
		AccessRelationFile:  accessRelationPath,
		AppIPFile:           appIPPath,
	})
	if err != nil {
		t.Fatalf("loadDataset returned error: %v", err)
	}
	report := buildReport(dataset, Options{BusinessNames: []string{"gdb-trans"}})
	if report.Summary.AuthorizationCount != 1 {
		t.Fatalf("expected 1 authorization row, got %d", report.Summary.AuthorizationCount)
	}
	if report.Rows[0].Privilege != "select,insert,update" {
		t.Fatalf("unexpected privilege: %q", report.Rows[0].Privilege)
	}
	if !reflect.DeepEqual(report.Rows[0].IPs, []string{"172.0.10.2", "172.0.10.3"}) {
		t.Fatalf("unexpected IPs: %#v", report.Rows[0].IPs)
	}
}

func TestReadTabularRecordsRejectsUnsupportedExtension(t *testing.T) {
	_, err := readTabularRecords(filepath.Join(t.TempDir(), "input.xls"))
	if err == nil || !strings.Contains(err.Error(), "unsupported table file extension") {
		t.Fatalf("expected unsupported extension error, got %v", err)
	}
}

func TestParseArgsRequiresOutputForCSV(t *testing.T) {
	_, err := parseArgs([]string{
		"--business-cluster-file", "a.csv",
		"--db-cluster-file", "b.csv",
		"--access-relation-file", "c.csv",
		"--app-ip-file", "d.csv",
		"--output-format", "csv",
	})
	if err == nil || !strings.Contains(err.Error(), "--output is required when --output-format csv") {
		t.Fatalf("expected csv output path error, got %v", err)
	}
}

func TestSplitBusinessNames(t *testing.T) {
	got := splitBusinessNames([]string{"gdb-trans,gdb-settle", "gdb-trans"})
	want := []string{"gdb-trans", "gdb-settle"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitBusinessNames mismatch, got %v want %v", got, want)
	}
}

func writeCSVRows(t *testing.T, path string, rows [][]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create csv fixture: %v", err)
	}
	writer := csv.NewWriter(file)
	if err := writer.WriteAll(rows); err != nil {
		_ = file.Close()
		t.Fatalf("write csv fixture: %v", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		t.Fatalf("flush csv fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close csv fixture: %v", err)
	}
}

func TestRenderCSVReport(t *testing.T) {
	report := Report{
		Rows: []ResultRow{
			{
				BusinessName:      "gdb-trans",
				DBType:            "goldendb",
				ClusterName:       "BJ13_clearing_branch_00",
				PrimaryHost:       "10.26.24.11",
				DBName:            "clearing_branch_00",
				ApplicationName:   "clearing-worker",
				ApplicationCenter: "13",
				DBPrimaryCenter:   "13",
				DBRole:            "主库",
				IPs:               []string{"172.0.10.2", "172.0.10.3"},
				DBUser:            "nclearingwork",
				Privilege:         "select,insert,update",
				MatchStatus:       "matched",
			},
		},
	}
	output, err := renderReport(report, "csv")
	if err != nil {
		t.Fatalf("renderReport returned error: %v", err)
	}
	if !strings.Contains(output, "业务名称,数据库类型,集群名") {
		t.Fatalf("csv header missing: %s", output)
	}
	if !strings.Contains(output, "\"172.0.10.2,172.0.10.3\"") {
		t.Fatalf("csv did not quote multi-ip cell: %s", output)
	}
	if !strings.Contains(output, "\"select,insert,update\"") {
		t.Fatalf("csv did not quote privilege cell: %s", output)
	}
}

func TestRenderDiagnostics(t *testing.T) {
	output := renderDiagnostics(Report{
		Diagnostics: []Diagnostic{
			{Type: "missing_cluster_mapping", Source: "cluster_01", Message: "cluster not found"},
		},
	})
	if !strings.Contains(output, "missing_cluster_mapping [cluster_01]: cluster not found") {
		t.Fatalf("diagnostics output mismatch: %s", output)
	}
}

func TestWriteXLSXReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.xlsx")
	report := Report{
		Rows: []ResultRow{
			{
				BusinessName:    "gdb-trans",
				DBType:          "goldendb",
				ClusterName:     "BJ13_clearing_branch_00",
				PrimaryHost:     "10.26.24.11",
				DBName:          "clearing_branch_00",
				ApplicationName: "clearing-worker",
				IPs:             []string{"172.0.10.2", "172.0.10.3"},
				DBUser:          "nclearingwork",
				Privilege:       "select,insert,update",
				MatchStatus:     "matched",
			},
		},
	}
	if err := writeXLSXReport(report, path); err != nil {
		t.Fatalf("writeXLSXReport returned error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("xlsx was not written: %v", err)
	}
	file, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open generated xlsx: %v", err)
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if !reflect.DeepEqual(sheets, []string{"授权明细"}) {
		t.Fatalf("unexpected sheets: %v", sheets)
	}
	detailValue, err := file.GetCellValue("授权明细", "J2")
	if err != nil {
		t.Fatalf("read detail cell: %v", err)
	}
	if detailValue != "172.0.10.2\n172.0.10.3" {
		t.Fatalf("unexpected IP cell: %q", detailValue)
	}
}
