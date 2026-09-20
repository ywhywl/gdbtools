package insightbatchdn

import "testing"

func TestNormalizeDNRowsUsesManualTeamIDAndDefaultPaths(t *testing.T) {
	rows, err := normalizeDNRows([]map[string]string{{
		"insight_addr": "127.0.0.1:8444", "cluster_name": "c1", "server_type": "vm_l",
		"dbgroup_id": "3", "team_id": "4", "ip": "10.0.0.1",
	}}, args{Prefix: "nu", BasePath: "/data/goldendb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d", len(rows))
	}
	row := rows[0]
	if row.TemplateName != "template_vm_l_dn.json" || row.TeamID != "4" {
		t.Fatalf("unexpected template or team: %#v", row)
	}
	if row.InstallUser != "nudb1" || row.InstallPath != "/data/goldendb/nudb1" || row.DataPath != "/data/goldendb/nudb1/data" || row.LogPath != "/data/goldendb/nudb1/log" {
		t.Fatalf("unexpected default paths: %#v", row)
	}
}

func TestNormalizeDNRowsRequiresTeamID(t *testing.T) {
	_, err := normalizeDNRows([]map[string]string{{
		"insight_addr": "127.0.0.1:8444", "cluster_name": "c1", "template_name": "template_vm_l_dn",
		"dbgroup_id": "3", "ip": "10.0.0.1",
	}}, args{})
	if err == nil {
		t.Fatal("expected missing team_id error")
	}
}

func TestBuildDNPayloadUsesDocumentedTemplateFileName(t *testing.T) {
	payload, err := buildDNPayload(nil, nil, 12, []dnRow{{
		DBGroupID: "3", TeamID: "1", IP: "10.0.0.41", Role: "M", TemplateName: "template_vm_l_dn.json", Port: "5501", AdminPort: "5502",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["parameterTemplateInfos"]; ok {
		t.Fatal("parameterTemplateInfos must not be included")
	}
	dbGroups := payload["dbgroupList"].([]map[string]any)
	teamList := dbGroups[0]["teamList"].([]map[string]any)
	dnList := teamList[0]["dnList"].([]map[string]any)
	if dnList[0]["port"] != 5501 || dnList[0]["adminPort"] != 5502 {
		t.Fatalf("unexpected dnList item: %#v", dnList[0])
	}
	if dnList[0]["templateName"] != "template_vm_l_dn.json" {
		t.Fatalf("unexpected node template: %#v", dnList[0]["templateName"])
	}
}

func TestBuildDNPayloadUsesOSTemplateOnOSNode(t *testing.T) {
	payload, err := buildDNPayload(nil, nil, 12, []dnRow{{
		DBGroupID: "3", TeamID: "1", IP: "10.0.0.42", Role: "OS", TemplateName: "template_vm_l_dn_OS.json",
	}})
	if err != nil {
		t.Fatal(err)
	}
	dbGroups := payload["dbgroupList"].([]map[string]any)
	dnList := dbGroups[0]["teamList"].([]map[string]any)[0]["dnList"].([]map[string]any)
	if dnList[0]["templateName"] != "template_vm_l_dn_OS.json" {
		t.Fatalf("unexpected OS node template: %#v", dnList[0]["templateName"])
	}
}
