package insightbatchcreate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderOutput(t *testing.T) {
	output := map[string]any{
		"success": true,
		"summary": map[string]any{
			"total":         1,
			"success_count": 1,
			"failed_count":  0,
		},
		"clusters": []map[string]any{{
			"cluster_name":       "cluster_01",
			"server_type":        "vm_l",
			"status":             "success",
			"task_id":            "task_01",
			"template_selection": map[string]any{},
			"error":              "",
		}},
	}

	t.Run("json writes to provided writer", func(t *testing.T) {
		var buffer bytes.Buffer
		renderOutput(&buffer, "json", output)
		if !strings.Contains(buffer.String(), `"success": true`) {
			t.Fatalf("expected JSON result, got %q", buffer.String())
		}
	})

	t.Run("text writes summary to provided writer", func(t *testing.T) {
		var buffer bytes.Buffer
		renderOutput(&buffer, "text", output)
		if got := buffer.String(); got != "总计 total=1 success=1 failed=0\n" {
			t.Fatalf("unexpected text output: %q", got)
		}
	})
}

func TestRenderTopLevelErrorWritesJSON(t *testing.T) {
	var buffer bytes.Buffer
	if err := renderTopLevelError(&buffer, errors.New("invalid input")); err != nil {
		t.Fatalf("renderTopLevelError returned error: %v", err)
	}
	if !strings.Contains(buffer.String(), `"error": "invalid input"`) {
		t.Fatalf("expected error JSON, got %q", buffer.String())
	}
}

func TestBuildCNInstallList(t *testing.T) {
	tests := []struct {
		name     string
		row      normalizedRow
		args     runArgs
		expected []map[string]any
	}{
		{
			name: "M role should have 3306 and 3307",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"M": "192.168.1.1",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				{
					"ip":          "192.168.1.1",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.1",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
			},
		},
		{
			name: "S role should have 3306 and 3307",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"S": "192.168.1.2",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				{
					"ip":          "192.168.1.2",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.2",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
			},
		},
		{
			name: "TS role should have 3306 and 3307",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"TS": "192.168.1.3",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				{
					"ip":          "192.168.1.3",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.3",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
			},
		},
		{
			name: "LS role should only have 3308",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"LS": "192.168.1.4",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				{
					"ip":          "192.168.1.4",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3308,
				},
			},
		},
		{
			name: "OS role should only have 3309",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"OS": "192.168.1.5",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				{
					"ip":          "192.168.1.5",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3309,
				},
			},
		},
		{
			name: "Multiple roles with correct ports",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"M":  "192.168.1.1",
					"S":  "192.168.1.2",
					"TS": "192.168.1.3",
					"LS": "192.168.1.4",
					"OS": "192.168.1.5",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				// M: 3306, 3307
				{
					"ip":          "192.168.1.1",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.1",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
				// S: 3306, 3307
				{
					"ip":          "192.168.1.2",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.2",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
				// LS: 3308
				{
					"ip":          "192.168.1.4",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3308,
				},
				// OS: 3309
				{
					"ip":          "192.168.1.5",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3309,
				},
				// TS: 3306, 3307
				{
					"ip":          "192.168.1.3",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.3",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
			},
		},
		{
			name: "Empty IP should be skipped",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"M":  "192.168.1.1",
					"S":  "",
					"TS": "",
					"LS": "192.168.1.4",
					"OS": "",
				},
			},
			args: runArgs{
				Prefix:   "nu",
				BasePath: "/data/goldendb",
			},
			expected: []map[string]any{
				// M: 3306, 3307
				{
					"ip":          "192.168.1.1",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3306,
				},
				{
					"ip":          "192.168.1.1",
					"installPath": "/data/goldendb/nudbproxy2",
					"installUser": "nudbproxy2",
					"servicePort": 3307,
				},
				// LS: 3308
				{
					"ip":          "192.168.1.4",
					"installPath": "/data/goldendb/nudbproxy1",
					"installUser": "nudbproxy1",
					"servicePort": 3308,
				},
			},
		},
		{
			name: "Custom prefix should work",
			row: normalizedRow{
				RoleIPs: map[string]string{
					"LS": "192.168.1.4",
					"OS": "192.168.1.5",
				},
			},
			args: runArgs{
				Prefix:   "test",
				BasePath: "/opt/goldendb",
			},
			expected: []map[string]any{
				{
					"ip":          "192.168.1.4",
					"installPath": "/opt/goldendb/testdbproxy1",
					"installUser": "testdbproxy1",
					"servicePort": 3308,
				},
				{
					"ip":          "192.168.1.5",
					"installPath": "/opt/goldendb/testdbproxy1",
					"installUser": "testdbproxy1",
					"servicePort": 3309,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCNInstallList(tt.row, tt.args)

			if len(result) != len(tt.expected) {
				t.Errorf("buildCNInstallList() returned %d items, expected %d", len(result), len(tt.expected))
				return
			}

			for i, item := range result {
				expected := tt.expected[i]

				if item["ip"] != expected["ip"] {
					t.Errorf("item[%d].ip = %v, want %v", i, item["ip"], expected["ip"])
				}
				if item["installPath"] != expected["installPath"] {
					t.Errorf("item[%d].installPath = %v, want %v", i, item["installPath"], expected["installPath"])
				}
				if item["installUser"] != expected["installUser"] {
					t.Errorf("item[%d].installUser = %v, want %v", i, item["installUser"], expected["installUser"])
				}
				if item["servicePort"] != expected["servicePort"] {
					t.Errorf("item[%d].servicePort = %v, want %v", i, item["servicePort"], expected["servicePort"])
				}
			}
		})
	}
}

func TestParseRoleIPs(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		role    string
		want    []string
		wantErr bool
	}{
		{name: "pipe and semicolon are equivalent", raw: "10.0.0.1;10.0.0.2|10.0.0.3", role: "S", want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
		{name: "single ip", raw: "10.0.0.1", role: "M", want: []string{"10.0.0.1"}},
		{name: "M cannot have multiple ips", raw: "10.0.0.1|10.0.0.2", role: "M", wantErr: true},
		{name: "empty item", raw: "10.0.0.1||10.0.0.2", role: "OS", wantErr: true},
		{name: "empty group", raw: "10.0.0.1;", role: "OS", wantErr: true},
		{name: "spaces are trimmed", raw: "10.0.0.1 | 10.0.0.2", role: "TS", want: []string{"10.0.0.1", "10.0.0.2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRoleIPs(tt.raw, tt.role)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRoleIPs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseRoleIPs() = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseRoleIPs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildDNInstallListWithExpandedRoles(t *testing.T) {
	row := normalizedRow{
		RoleIPLists: map[string][]string{
			"M":  {"10.0.0.1"},
			"S":  {"10.0.0.2", "10.0.0.3"},
			"OS": {"10.0.0.4", "10.0.0.5", "10.0.0.6"},
			"TS": {"10.0.0.7", "10.0.0.8"},
		},
		Templates: templateSelection{DnOSTemplate: "template_vm_l_dn_OS.json"},
	}
	items := buildDNInstallList(row, runArgs{Prefix: "nu", BasePath: "/data/goldendb"})[0]["teamList"].([]map[string]any)
	want := []struct {
		teamID int
		ip     string
		dbRole int
		tpl    string
	}{
		{1, "10.0.0.1", 1, ""},
		{2, "10.0.0.2", 0, ""},
		{3, "10.0.0.3", 0, ""},
		{4, "10.0.0.4", 2, "template_vm_l_dn_OS.json"},
		{5, "10.0.0.5", 0, "template_vm_l_dn_OS.json"},
		{6, "10.0.0.6", 0, "template_vm_l_dn_OS.json"},
		{7, "10.0.0.7", 0, ""},
		{8, "10.0.0.8", 0, ""},
	}
	if len(items) != len(want) {
		t.Fatalf("team count = %d, want %d", len(items), len(want))
	}
	for i, item := range items {
		if got := item["teamId"]; got != want[i].teamID {
			t.Errorf("team[%d] teamId = %v, want %d", i, got, want[i].teamID)
		}
		dn := item["dnList"].([]map[string]any)[0]
		if got := dn["ip"]; got != want[i].ip {
			t.Errorf("team[%d] ip = %v, want %s", i, got, want[i].ip)
		}
		if got := dn["dbRole"]; got != want[i].dbRole {
			t.Errorf("team[%d] dbRole = %v, want %d", i, got, want[i].dbRole)
		}
		if want[i].tpl == "" {
			if _, ok := dn["templateName"]; ok {
				t.Errorf("team[%d] unexpected templateName", i)
			}
		} else if got := dn["templateName"]; got != want[i].tpl {
			t.Errorf("team[%d] templateName = %v, want %s", i, got, want[i].tpl)
		}
	}
}

func TestLoadRowsTeamLimits(t *testing.T) {
	const header = "num,cluster_name,cluster_group_name,M,S,TS,LS,OS,server_type\n"
	const tenIPs = "1,cluster_10,group,10.0.0.1,10.0.0.2;10.0.0.3|10.0.0.4,10.0.0.9|10.0.0.10,10.0.0.5;10.0.0.6,10.0.0.7|10.0.0.8,vm_l\n"
	const elevenIPs = "2,cluster_11,group,10.0.0.1,10.0.0.2;10.0.0.3|10.0.0.4,10.0.0.9|10.0.0.10;10.0.0.11,10.0.0.5;10.0.0.6,10.0.0.7|10.0.0.8,vm_l\n"
	tests := []struct {
		name       string
		csvRows    string
		wantCounts []int
		wantError  string
	}{
		{
			name:       "single IP per role",
			csvRows:    "1,cluster_5,group,10.0.0.1,10.0.0.2,10.0.0.5,10.0.0.3,10.0.0.4,vm_l\n",
			wantCounts: []int{5},
		},
		{
			name:       "missing role does not reserve a team ID",
			csvRows:    "1,cluster_4,group,10.0.0.1,10.0.0.2,10.0.0.4,,10.0.0.3,vm_l\n",
			wantCounts: []int{4},
		},
		{
			name:       "ten IPs across roles",
			csvRows:    tenIPs,
			wantCounts: []int{10},
		},
		{
			name:       "nine S IPs fill all remaining teams",
			csvRows:    "1,cluster_10,group,10.0.0.1,10.0.0.2|10.0.0.3|10.0.0.4|10.0.0.5|10.0.0.6|10.0.0.7|10.0.0.8|10.0.0.9|10.0.0.10,,,,vm_l\n",
			wantCounts: []int{10},
		},
		{
			name:       "limit and numbering reset per cluster",
			csvRows:    tenIPs + strings.ReplaceAll(tenIPs, "cluster_10", "another_cluster"),
			wantCounts: []int{10, 10},
		},
		{
			name:      "eleven IPs reject the entire batch",
			csvRows:   tenIPs + elevenIPs,
			wantError: "第 2 行集群 cluster_11 IP 数量为 11，最多支持 10 个 IP",
		},
	}
	for _, tt := range tests {
		for _, autoSelect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/autoSelect=%t", tt.name, autoSelect), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "clusters.csv")
				if err := os.WriteFile(path, []byte(header+tt.csvRows), 0o600); err != nil {
					t.Fatal(err)
				}
				rows, err := loadRows(path, autoSelect)
				if tt.wantError != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("loadRows() error = %v, want %q", err, tt.wantError)
					}
					if rows != nil {
						t.Fatal("invalid batch must not return partially validated rows")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != len(tt.wantCounts) {
					t.Fatalf("row count = %d, want %d", len(rows), len(tt.wantCounts))
				}
				for rowIndex, row := range rows {
					payload := buildPayload(row, runArgs{Prefix: "nu", BasePath: "/data/goldendb"}, "")
					groups := payload["dnInstallList"].([]map[string]any)
					teams := groups[0]["teamList"].([]map[string]any)
					if len(teams) != tt.wantCounts[rowIndex] {
						t.Fatalf("row %d team count = %d, want %d", rowIndex, len(teams), tt.wantCounts[rowIndex])
					}
					for i, team := range teams {
						if team["teamId"] != i+1 {
							t.Errorf("team[%d].teamId = %v, want %d", i, team["teamId"], i+1)
						}
						dns := team["dnList"].([]map[string]any)
						if len(dns) != 1 {
							t.Fatalf("team[%d] DN count = %d, want 1", i, len(dns))
						}
						if wantIP := fmt.Sprintf("10.0.0.%d", i+1); dns[0]["ip"] != wantIP {
							t.Errorf("team[%d] IP = %v, want %s", i, dns[0]["ip"], wantIP)
						}
					}
				}
			})
		}
	}
}

func TestMemToServerTypeLowMemoryErrors(t *testing.T) {
	for _, mem := range []int{0, 1, 21} {
		got, err := memToServerType(mem, "kvm", false)
		if err == nil || got != "" {
			t.Errorf("memToServerType(%d) = %q, %v; want error", mem, got, err)
		}
	}
}

func TestMemToServerTypeThreshold(t *testing.T) {
	for _, mem := range []int{22, 23} {
		got, err := memToServerType(mem, "kvm", false)
		if err != nil || got != "vm_l" {
			t.Errorf("memToServerType(%d) = %q, %v; want vm_l, nil", mem, got, err)
		}
	}
}

func TestMemToServerTypeLowMemoryCanBeAllowed(t *testing.T) {
	got, err := memToServerType(21, "kvm", true)
	if err != nil || got != "vm_l" {
		t.Fatalf("memToServerType() = %q, %v; want vm_l, nil", got, err)
	}
}

func TestSelectClusterServerType(t *testing.T) {
	tests := []struct {
		name      string
		typeName  string
		mismatch  bool
		lowMemory bool
		allow     bool
		want      string
		wantErr   bool
	}{
		{name: "low memory without mismatch remains an error", typeName: "vm_l", lowMemory: true, allow: true, wantErr: true},
		{name: "low memory with mismatch uses vm_l", typeName: "vm_m", mismatch: true, lowMemory: true, allow: true, want: "vm_l"},
		{name: "low memory mismatch without allow is an error", typeName: "vm_m", mismatch: true, lowMemory: true, wantErr: true},
		{name: "normal mismatch with allow uses actual baseline", typeName: "vm_m", mismatch: true, allow: true, want: "vm_m"},
		{name: "normal mismatch without allow is an error", typeName: "vm_m", mismatch: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectClusterServerType(tt.typeName, tt.mismatch, tt.lowMemory, tt.allow)
			if (err != nil) != tt.wantErr {
				t.Fatalf("selectClusterServerType() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("selectClusterServerType() = %q, want %q", got, tt.want)
			}
		})
	}
}
