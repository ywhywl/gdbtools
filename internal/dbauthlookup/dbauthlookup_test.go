package dbauthlookup

import (
	"reflect"
	"testing"
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

func TestBuildReportWithFallbackClusterDBName(t *testing.T) {
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
	report := buildReport(dataset, Options{BusinessName: "gdb-trans", WithDiagnostics: true})
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
