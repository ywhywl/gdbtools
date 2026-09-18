package insightopen

import "testing"

func TestNormalizeParameterTemplateName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "template_vm_l_cn", want: "template_vm_l_cn.json"},
		{name: "template_vm_l_dn.json", want: "template_vm_l_dn.json"},
		{name: "TEMPLATE.JSON", want: "TEMPLATE.JSON"},
		{name: "  template.json  ", want: "template.json"},
		{name: "", want: ""},
	}

	for _, tt := range tests {
		if got := NormalizeParameterTemplateName(tt.name); got != tt.want {
			t.Errorf("NormalizeParameterTemplateName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
