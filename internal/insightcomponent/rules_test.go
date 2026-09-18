package insightcomponent

import "testing"

func TestComponentTemplateName(t *testing.T) {
	tests := []struct {
		explicit      string
		serverType    string
		component     string
		caseSensitive bool
		want          string
	}{
		{"template_vm_l_cn", "vm_m", "cn", false, "template_vm_l_cn.json"},
		{"", "vm_l", "dn", false, "template_vm_l_dn.json"},
		{"", "vm_l", "cn", true, "template_vm_l_lowercase_0_cn.json"},
		{"", "vm_lowercase_0", "cn", true, "template_vm_lowercase_0_lowercase_0_cn.json"},
	}
	for _, tt := range tests {
		got, err := ComponentTemplateName(tt.explicit, tt.serverType, tt.component, tt.caseSensitive)
		if err != nil || got != tt.want {
			t.Errorf("ComponentTemplateName(%q, %q, %q, %t) = %q, %v; want %q", tt.explicit, tt.serverType, tt.component, tt.caseSensitive, got, err, tt.want)
		}
	}
}
