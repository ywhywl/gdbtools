package insightcomponent

import (
	"fmt"
	"strconv"
	"strings"
)

var supportedServerTypes = map[string]struct{}{
	"vm_l":           {},
	"vm_m":           {},
	"vm_h":           {},
	"pm":             {},
	"vm_lowercase_0": {},
}

var supportedRoles = map[string]struct{}{
	"M":  {},
	"S":  {},
	"TS": {},
	"LS": {},
	"OS": {},
}

// ComponentTemplateName returns the explicit template name when supplied, or
// derives the same template file name used by insight-batch-create.
func ComponentTemplateName(explicit, serverType, component string, caseSensitive bool) (string, error) {
	if name := normalizeTemplateName(explicit); name != "" {
		return name, nil
	}

	serverType = strings.TrimSpace(serverType)
	if serverType == "" {
		return "", fmt.Errorf("template_name 或 server_type 至少提供一个")
	}
	if _, ok := supportedServerTypes[serverType]; !ok {
		return "", fmt.Errorf("不支持的 server_type: %s，当前仅支持 pm, vm_h, vm_l, vm_m, vm_lowercase_0", serverType)
	}

	component = strings.TrimSpace(component)
	if component == "" {
		return "", fmt.Errorf("组件类型不能为空")
	}
	serverPart := serverType
	if caseSensitive {
		serverPart += "_lowercase_0"
	}
	return fmt.Sprintf("template_%s_%s.json", serverPart, component), nil
}

func normalizeTemplateName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" && !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	return name
}

func NormalizeRole(role string) (string, error) {
	role = strings.ToUpper(strings.TrimSpace(role))
	if role == "" {
		return "", nil
	}
	if _, ok := supportedRoles[role]; !ok {
		return "", fmt.Errorf("不支持的 role: %s，当前仅支持 M, S, TS, LS, OS", role)
	}
	return role, nil
}

// DefaultCNServicePorts returns the service ports used by batch create for a
// component role.
func DefaultCNServicePorts(role string) ([]int, error) {
	role, err := NormalizeRole(role)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, fmt.Errorf("未提供 role，无法推导 CN service_port")
	}
	switch role {
	case "LS":
		return []int{3308}, nil
	case "OS":
		return []int{3309}, nil
	default:
		return []int{3306, 3307}, nil
	}
}

func RoleAllowsCNServicePort(role string, port int) bool {
	role = strings.ToUpper(strings.TrimSpace(role))
	switch role {
	case "M", "S", "TS":
		return port == 3306 || port == 3307
	case "LS":
		return port == 3308
	case "OS":
		return port == 3309
	default:
		return false
	}
}

func CNInstallUser(prefix string, servicePort int) (string, bool) {
	suffix := 0
	switch servicePort {
	case 3306, 3308, 3309:
		suffix = 1
	case 3307:
		suffix = 2
	default:
		return "", false
	}
	return strings.TrimSpace(prefix) + "dbproxy" + strconv.Itoa(suffix), true
}

func DefaultDNInstallUser(prefix string) string {
	return strings.TrimSpace(prefix) + "db1"
}

func InstallPath(basePath, installUser string) string {
	basePath = strings.TrimRight(strings.TrimSpace(basePath), "/")
	installUser = strings.Trim(strings.TrimSpace(installUser), "/")
	if basePath == "" {
		return installUser
	}
	if installUser == "" {
		return basePath
	}
	return basePath + "/" + installUser
}
