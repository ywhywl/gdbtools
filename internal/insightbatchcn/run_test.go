package insightbatchcn

import "testing"

func TestNormalizeCNRowsExpandsManualServicePorts(t *testing.T) {
	rows, err := normalizeCNRows([]map[string]string{{
		"insight_addr": "127.0.0.1:8444", "cluster_name": "c1", "template_name": "template_vm_l_cn",
		"role": "M", "ip": "10.0.0.1", "service_port": "3306|3307",
	}}, args{Prefix: "nu", BasePath: "/data/goldendb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ServicePort != "3306" || rows[1].ServicePort != "3307" {
		t.Fatalf("unexpected expanded rows: %#v", rows)
	}
	if rows[0].InstallUser != "nudbproxy1" || rows[1].InstallUser != "nudbproxy2" {
		t.Fatalf("unexpected install users: %#v", rows)
	}
}

func TestNormalizeCNRowsUsesRoleDefaults(t *testing.T) {
	rows, err := normalizeCNRows([]map[string]string{{
		"insight_addr": "127.0.0.1:8444", "cluster_name": "c1", "server_type": "vm_l",
		"role": "LS", "ip": "10.0.0.1",
	}}, args{Prefix: "nu", BasePath: "/data/goldendb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TemplateName != "template_vm_l_cn.json" || rows[0].ServicePort != "3308" {
		t.Fatalf("unexpected role defaults: %#v", rows)
	}
	if rows[0].InstallPath != "/data/goldendb/nudbproxy1" {
		t.Fatalf("install path = %q", rows[0].InstallPath)
	}
}

func TestNormalizeCNRowsRejectsRolePortMismatch(t *testing.T) {
	_, err := normalizeCNRows([]map[string]string{{
		"insight_addr": "127.0.0.1:8444", "cluster_name": "c1", "template_name": "template_vm_l_cn",
		"role": "M", "ip": "10.0.0.1", "service_port": "3308",
	}}, args{})
	if err == nil {
		t.Fatal("expected role/port mismatch error")
	}
}

func TestNormalizeCNRowsRequiresUserForNonStandardPort(t *testing.T) {
	_, err := normalizeCNRows([]map[string]string{{
		"insight_addr": "127.0.0.1:8444", "cluster_name": "c1", "template_name": "template_vm_l_cn",
		"role": "M", "ip": "10.0.0.1", "service_port": "9999",
	}}, args{AllowRolePortMismatch: true})
	if err == nil {
		t.Fatal("expected non-standard port install user error")
	}
}

func TestParseServicePortsRejectsEmptyItem(t *testing.T) {
	if _, err := parseServicePorts("3306;|3307"); err == nil {
		t.Fatal("expected empty service port item error")
	}
}

func TestBuildCNPayloadUsesDocumentedTemplateFileName(t *testing.T) {
	payload := buildCNPayload(12, "template_vm_l_cn.json", []cnRow{{
		IP: "10.0.0.31", Port: "5501", InstallUser: "nudbproxy1", InstallPath: "/data/gdb", ServicePort: "3306",
	}})

	if got := payload["clusterId"]; got != 12 {
		t.Fatalf("clusterId = %v, want 12", got)
	}
	templates := payload["parameterTemplateInfos"].([]map[string]any)
	if len(templates) != 1 || templates[0]["type"] != "CN" || templates[0]["templateName"] != "template_vm_l_cn.json" {
		t.Fatalf("unexpected parameterTemplateInfos: %#v", templates)
	}
	if got := payload["cnList"].([]map[string]any)[0]; got["port"] != 5501 || got["servicePort"] != 3306 {
		t.Fatalf("unexpected cnList item: %#v", got)
	}
}
