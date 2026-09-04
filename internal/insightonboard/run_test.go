package insightonboard

import (
	"encoding/json"
	"testing"

	"github.com/ywhywl/gdbtools/internal/hostchecker"
)

func TestHostTaskResponseFailedIPAcceptsString(t *testing.T) {
	var response hostTaskResponse
	err := json.Unmarshal([]byte(`{
		"result": "fail",
		"failedIp": "10.0.0.21"
	}`), &response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(response.FailedIP) != 1 || response.FailedIP[0] != "10.0.0.21" {
		t.Fatalf("unexpected failedIp: %#v", response.FailedIP)
	}
}

func TestHostTaskResponseFailedIPAcceptsArray(t *testing.T) {
	var response hostTaskResponse
	err := json.Unmarshal([]byte(`{
		"result": "fail",
		"failedIp": ["10.0.0.21", "10.0.0.22"]
	}`), &response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(response.FailedIP) != 2 ||
		response.FailedIP[0] != "10.0.0.21" ||
		response.FailedIP[1] != "10.0.0.22" {
		t.Fatalf("unexpected failedIp: %#v", response.FailedIP)
	}
}

func TestExtractStringSliceAcceptsDataArray(t *testing.T) {
	var response map[string]any
	err := json.Unmarshal([]byte(`{
		"code": 2,
		"msg": "fail",
		"data": ["主机名超过了40个字符"]
	}`), &response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := extractStringSlice(response["data"])
	if len(data) != 1 || data[0] != "主机名超过了40个字符" {
		t.Fatalf("unexpected data: %#v", data)
	}
}

func TestBuildPathResolveOutput(t *testing.T) {
	output := buildPathResolveOutput([]hostchecker.CheckResult{
		{
			IP:                  "10.0.0.21",
			ResolvedDataPath:    "/data",
			ResolvedInstallPath: "/data",
		},
	})

	if len(output) != 1 {
		t.Fatalf("unexpected output length: %d", len(output))
	}
	if output[0]["ip"] != "10.0.0.21" ||
		output[0]["data_path"] != "/data" ||
		output[0]["install_path"] != "/data" {
		t.Fatalf("unexpected path resolve output: %#v", output)
	}
	if _, ok := output[0]["passed"]; ok {
		t.Fatalf("path resolve output must not contain passed: %#v", output)
	}
}
