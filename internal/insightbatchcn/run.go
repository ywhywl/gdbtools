package insightbatchcn

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ywhywl/gdbtools/internal/insightcomponent"
	"github.com/ywhywl/gdbtools/internal/insightinput"
	"github.com/ywhywl/gdbtools/internal/insightopen"
)

type args struct {
	Input                 string
	Auth                  insightopen.AuthFlags
	DefaultPort           string
	DefaultInstallUser    string
	DefaultInstallPath    string
	DefaultServicePort    string
	Prefix                string
	BasePath              string
	CaseSensitive         bool
	AllowRolePortMismatch bool
	PollInterval          int
	PollTimeout           int
	VerifySSL             bool
	OutputJSON            bool
	Debug                 bool
}

func Run(argv []string) (int, error) {
	fs := flag.NewFlagSet("insight-batch-add-cn", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var parsed args
	fs.StringVar(&parsed.Input, "input", "", "输入文件路径 (CSV 或 JSON)")
	fs.StringVar(&parsed.DefaultPort, "default-port", "", "默认 CN 端口")
	fs.StringVar(&parsed.DefaultInstallUser, "default-install-user", "", "默认安装用户")
	fs.StringVar(&parsed.DefaultInstallPath, "default-install-path", "", "默认安装路径")
	fs.StringVar(&parsed.DefaultServicePort, "default-service-port", "", "默认服务端口")
	fs.StringVar(&parsed.Prefix, "prefix", "nu", "自动生成安装用户名的前缀")
	fs.StringVar(&parsed.BasePath, "base-path", "/data/goldendb", "自动生成安装路径的根目录")
	fs.BoolVar(&parsed.CaseSensitive, "case-sensitive", false, "使用大小写敏感模板名")
	fs.BoolVar(&parsed.AllowRolePortMismatch, "allow-role-port-mismatch", false, "允许 role 与 service_port 不匹配")
	fs.IntVar(&parsed.PollInterval, "poll-interval", 10, "轮询间隔")
	fs.IntVar(&parsed.PollTimeout, "poll-timeout", 3600, "轮询超时")
	fs.BoolVar(&parsed.VerifySSL, "verify-ssl", false, "启用 SSL 证书校验；默认关闭")
	fs.BoolVar(&parsed.OutputJSON, "output-json", false, "输出 JSON")
	fs.BoolVar(&parsed.Debug, "debug", false, "打印请求和响应的 debug 日志；默认关闭")
	insightopen.AddAuthFlags(fs, &parsed.Auth)
	if err := fs.Parse(argv); err != nil {
		return 2, err
	}
	insightopen.SetDebug(parsed.Debug)
	if strings.TrimSpace(parsed.Input) == "" {
		return 2, fmt.Errorf("--input is required")
	}

	rows, err := insightinput.ParseTabularInput(parsed.Input)
	if err != nil {
		return 2, err
	}
	normalized, err := normalizeCNRows(rows, parsed)
	if err != nil {
		return 2, err
	}

	grouped := groupCNRows(normalized)
	groupResults := make([]map[string]any, 0, len(grouped))
	itemResults := make([]map[string]any, 0, len(normalized))

	for _, group := range grouped {
		auth, err := insightopen.ResolveAuth(parsed.Auth)
		if err != nil {
			return 2, err
		}
		client, err := insightopen.NewClient(group.APIBase, !parsed.VerifySSL, auth)
		if err != nil {
			return 2, err
		}
		clusterID, err := insightopen.SearchClusterID(context.Background(), client, group.ClusterName)
		if err != nil {
			return 2, err
		}
		taskID, err := insightopen.StartInstallTask(context.Background(), client, "/open_api/insight/external/install/batchAddCN", buildCNPayload(clusterID, group.TemplateName, group.Rows))
		if err != nil {
			return 2, err
		}
		finalData, err := insightopen.PollTaskResult(context.Background(), client, "/open_api/insight/external/install/querybatchAddCNResult", taskID, insightopen.PollOptions{
			Interval: time.Duration(parsed.PollInterval) * time.Second,
			Timeout:  time.Duration(parsed.PollTimeout) * time.Second,
		})
		if err != nil {
			return 2, err
		}

		records := toRecords(finalData["records"])
		groupItems := summarizeCNGroup(records, group.Rows)
		for _, item := range groupItems {
			itemResults = append(itemResults, mergeMaps(map[string]any{
				"insight_addr":  group.APIBase,
				"cluster_name":  group.ClusterName,
				"template_name": group.TemplateName,
			}, item))
		}
		successCount, failedCount := countStatuses(groupItems)
		groupResults = append(groupResults, map[string]any{
			"insight_addr":  group.APIBase,
			"cluster_name":  group.ClusterName,
			"template_name": group.TemplateName,
			"task_id":       taskID,
			"total":         len(groupItems),
			"success_count": successCount,
			"failed_count":  failedCount,
			"status":        groupStatus(successCount, failedCount),
			"items":         groupItems,
		})
	}

	output := map[string]any{
		"summary": map[string]any{
			"total":         len(itemResults),
			"success_count": countResultStatus(itemResults, "success"),
			"failed_count":  countResultStatus(itemResults, "failed"),
		},
		"groups":  groupResults,
		"results": itemResults,
	}

	if parsed.OutputJSON {
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	} else {
		log.Printf("总计 %v", output["summary"])
	}
	return 0, nil
}

type cnRow struct {
	RowNo        int
	APIBase      string
	ClusterName  string
	TemplateName string
	IP           string
	Port         string
	InstallUser  string
	InstallPath  string
	ServicePort  string
	Role         string
}

type cnGroup struct {
	APIBase      string
	ClusterName  string
	TemplateName string
	Rows         []cnRow
}

func normalizeCNRows(rows []map[string]string, args args) ([]cnRow, error) {
	out := make([]cnRow, 0, len(rows))
	for i, row := range rows {
		api := strings.TrimSpace(row["insight_addr"])
		cluster := strings.TrimSpace(row["cluster_name"])
		serverType := strings.TrimSpace(row["server_type"])
		role, err := insightcomponent.NormalizeRole(row["role"])
		if err != nil {
			return nil, fmt.Errorf("第 %d 行: %w", i+1, err)
		}
		template, err := insightcomponent.ComponentTemplateName(row["template_name"], serverType, "cn", args.CaseSensitive)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行: %w", i+1, err)
		}
		ip := strings.TrimSpace(row["ip"])
		if api == "" || cluster == "" || ip == "" {
			return nil, fmt.Errorf("第 %d 行缺少必填字段 insight_addr/cluster_name/template_name/ip", i+1)
		}
		apiBase, err := insightopen.NormalizeAPIBase(api)
		if err != nil {
			return nil, err
		}
		servicePortText := firstNonEmpty(strings.TrimSpace(row["service_port"]), strings.TrimSpace(args.DefaultServicePort))
		servicePorts, err := parseServicePorts(servicePortText)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 service_port: %w", i+1, err)
		}
		if len(servicePorts) == 0 {
			servicePorts, err = insightcomponent.DefaultCNServicePorts(role)
			if err != nil {
				return nil, fmt.Errorf("第 %d 行: %w", i+1, err)
			}
		}
		manualUser := firstNonEmpty(strings.TrimSpace(row["install_user"]), strings.TrimSpace(args.DefaultInstallUser))
		manualPath := firstNonEmpty(strings.TrimSpace(row["install_path"]), strings.TrimSpace(args.DefaultInstallPath))
		for _, servicePort := range servicePorts {
			if role != "" && !args.AllowRolePortMismatch && !insightcomponent.RoleAllowsCNServicePort(role, servicePort) {
				return nil, fmt.Errorf("第 %d 行 role=%s 不允许 service_port=%d；使用 --allow-role-port-mismatch 放宽校验", i+1, role, servicePort)
			}
			installUser := manualUser
			if installUser == "" {
				var ok bool
				installUser, ok = insightcomponent.CNInstallUser(args.Prefix, servicePort)
				if !ok {
					return nil, fmt.Errorf("第 %d 行 service_port=%d 为非标准端口，必须人工填写 install_user", i+1, servicePort)
				}
			}
			installPath := manualPath
			if installPath == "" {
				installPath = insightcomponent.InstallPath(args.BasePath, installUser)
			}
			out = append(out, cnRow{
				RowNo:        i + 1,
				APIBase:      apiBase,
				ClusterName:  cluster,
				TemplateName: template,
				IP:           ip,
				Port:         firstNonEmpty(strings.TrimSpace(row["port"]), args.DefaultPort),
				InstallUser:  installUser,
				InstallPath:  installPath,
				ServicePort:  strconv.Itoa(servicePort),
				Role:         role,
			})
		}
	}
	return out, nil
}

func parseServicePorts(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.Contains(value, ",") {
		return nil, fmt.Errorf("仅支持使用 ';' 或 '|' 分隔多个端口")
	}
	parts := strings.Split(strings.ReplaceAll(value, "|", ";"), ";")
	ports := make([]int, 0, len(parts))
	seen := map[int]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("端口列表包含空项")
		}
		port, err := strconv.Atoi(part)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("无效端口 %q", part)
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	return ports, nil
}

func groupCNRows(rows []cnRow) []cnGroup {
	index := map[string]int{}
	out := make([]cnGroup, 0)
	for _, row := range rows {
		key := row.APIBase + "\x00" + row.ClusterName + "\x00" + row.TemplateName
		if pos, ok := index[key]; ok {
			out[pos].Rows = append(out[pos].Rows, row)
			continue
		}
		index[key] = len(out)
		out = append(out, cnGroup{APIBase: row.APIBase, ClusterName: row.ClusterName, TemplateName: row.TemplateName, Rows: []cnRow{row}})
	}
	return out
}

func buildCNPayload(clusterID int, templateName string, rows []cnRow) map[string]any {
	cnList := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{"ip": row.IP}
		if row.Port != "" {
			item["port"] = mustInt(row.Port)
		}
		if row.InstallUser != "" {
			item["installUser"] = row.InstallUser
		}
		if row.InstallPath != "" {
			item["installPath"] = row.InstallPath
		}
		if row.ServicePort != "" {
			item["servicePort"] = mustInt(row.ServicePort)
		}
		cnList = append(cnList, item)
	}
	return map[string]any{
		"clusterId": clusterID,
		"parameterTemplateInfos": []map[string]any{
			{"type": "CN", "templateName": templateName},
		},
		"cnList": cnList,
	}
}

func summarizeCNGroup(records []map[string]any, rows []cnRow) []map[string]any {
	recordMap := map[string]map[string]any{}
	for _, item := range records {
		key := toString(item["ip"]) + "\x00" + toString(item["port"])
		recordMap[key] = item
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		key := row.IP + "\x00" + row.Port
		matched := recordMap[key]
		result := "success"
		message := ""
		port := row.Port
		if matched != nil {
			if toString(matched["result"]) != "" {
				result = toString(matched["result"])
			}
			message = toString(matched["failMsg"])
			if toString(matched["port"]) != "" {
				port = toString(matched["port"])
			}
		}
		status := "success"
		if result != "success" {
			status = "failed"
		}
		out = append(out, map[string]any{
			"row_no":  row.RowNo,
			"ip":      row.IP,
			"port":    port,
			"status":  status,
			"message": message,
		})
	}
	return out
}

func toRecords(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if typed, ok := item.(map[string]any); ok {
			out = append(out, typed)
		}
	}
	return out
}

func mergeMaps(left, right map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}

func countStatuses(items []map[string]any) (int, int) {
	success := 0
	for _, item := range items {
		if toString(item["status"]) == "success" {
			success++
		}
	}
	return success, len(items) - success
}

func countResultStatus(items []map[string]any, status string) int {
	count := 0
	for _, item := range items {
		if toString(item["status"]) == status {
			count++
		}
	}
	return count
}

func groupStatus(successCount, failedCount int) string {
	if failedCount == 0 {
		return "success"
	}
	if successCount > 0 {
		return "partial_success"
	}
	return "failed"
}

func mustInt(value string) int {
	out, _ := strconv.Atoi(value)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
