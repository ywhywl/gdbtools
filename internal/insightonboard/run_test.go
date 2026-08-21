package insightonboard

import (
	"encoding/json"
	"testing"
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
