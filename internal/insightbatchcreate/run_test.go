package insightbatchcreate

import (
	"testing"
)

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
