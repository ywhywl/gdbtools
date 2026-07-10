package insightbatchcreate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ywhywl/gdbtools/internal/insightinput"
	"github.com/ywhywl/gdbtools/internal/insightopen"
)

var (
	supportedServerTypes = map[string]struct{}{
		"vm_l":           {},
		"vm_m":           {},
		"vm_h":           {},
		"pm":             {},
		"vm_lowercase_0": {},
	}
	requiredHeaders = []string{"num", "cluster_name", "cluster_group_name", "M", "S", "TS", "LS", "OS", "server_type"}
	roleSequence    = []string{"M", "S", "LS", "OS", "TS"}
	roleToTeamID    = map[string]int{"M": 1, "S": 2, "LS": 3, "OS": 4, "TS": 5}
	roleToDBRole    = map[string]int{"M": 1, "S": 0, "TS": 0, "LS": 0, "OS": 2}
)

type templateSelection struct {
	ServerType     string `json:"server_type"`
	GlobalTemplate string `json:"global_template"`
	DNTemplate     string `json:"dn_template"`
	CNTemplate     string `json:"cn_template"`
}

type normalizedRow struct {
	RowNo            int
	Num              string
	ClusterName      string
	ClusterGroupName string
	RoleIPs          map[string]string
	ServerType       string
	Templates        templateSelection
}

type runArgs struct {
	API            string
	CSV            string
	Auth           insightopen.AuthFlags
	Prefix         string
	BasePath       string
	InsUserPwd     string
	InsUserPwdB64  string
	HAMode         int
	InstanceType   int
	Charset        string
	Mode           int
	GTMUseMode     int
	ClusterDesc    string
	WaitCompletion bool
	MaxWaitTime    int
	PollInterval   int
	MaxRetries     int
	NoVerify       bool
	DryRun         bool
	Output         string
	Format         string
}

type clusterProgress struct {
	Result    string `json:"result"`
	Process   any    `json:"process"`
	ErrorCode any    `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
}

func Run(args []string) (int, error) {
	log.SetFlags(log.LstdFlags)

	parsedArgs, err := parseArgs(args)
	if err != nil {
		return 3, nil
	}

	auth, err := insightopen.ResolveAuth(parsedArgs.Auth)
	if err != nil {
		return 3, renderTopLevelError(err)
	}

	client, err := insightopen.NewClient(parsedArgs.API, parsedArgs.NoVerify, auth)
	if err != nil {
		return 3, renderTopLevelError(err)
	}
	passwordB64, err := resolvePasswordB64(parsedArgs.InsUserPwd, parsedArgs.InsUserPwdB64)
	if err != nil {
		return 3, renderTopLevelError(err)
	}
	rows, err := loadRows(parsedArgs.CSV)
	if err != nil {
		return 3, renderTopLevelError(err)
	}
	if parsedArgs.GTMUseMode != 1 {
		return 3, renderTopLevelError(fmt.Errorf("第一版仅支持 --gtm-use-mode=1"))
	}

	logTemplateSelection(rows)

	clusters := make([]map[string]any, 0, len(rows))
	if parsedArgs.DryRun {
		for _, row := range rows {
			payload := buildPayload(row, parsedArgs, passwordB64)
			clusters = append(clusters, map[string]any{
				"row_no":             row.RowNo,
				"num":                row.Num,
				"cluster_name":       row.ClusterName,
				"cluster_group_name": row.ClusterGroupName,
				"server_type":        row.ServerType,
				"template_selection": buildTemplateSelectionOutput(row),
				"status":             "dry_run",
				"task_id":            "",
				"attempt":            0,
				"request_payload":    payload,
				"response":           nil,
				"error":              "",
			})
		}
	} else {
		for _, row := range rows {
			payload := buildPayload(row, parsedArgs, passwordB64)
			clusters = append(clusters, executeRow(context.Background(), client, row, payload, parsedArgs))
		}
	}

	successCount := 0
	failedCount := 0
	for _, item := range clusters {
		status := fmt.Sprint(item["status"])
		if status == "success" || status == "dry_run" {
			successCount++
		}
		if status == "failed" {
			failedCount++
		}
	}

	output := map[string]any{
		"success": failedCount == 0,
		"api":     client.BaseURL(),
		"summary": map[string]any{
			"total":         len(clusters),
			"success_count": successCount,
			"failed_count":  failedCount,
		},
		"clusters": clusters,
	}

	if err := writeOutput(parsedArgs.Output, output); err != nil {
		return 3, renderTopLevelError(err)
	}
	renderStdout(parsedArgs.Format, output)

	if parsedArgs.DryRun || failedCount == 0 {
		return 0, nil
	}
	if successCount > 0 {
		return 1, nil
	}
	return 2, nil
}

func parseArgs(args []string) (runArgs, error) {
	fs := flag.NewFlagSet("insight-batch-create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	parsed := runArgs{}
	fs.StringVar(&parsed.API, "api", "", "Insight 地址")
	fs.StringVar(&parsed.CSV, "csv", "", "CSV 文件路径")
	fs.StringVar(&parsed.Prefix, "prefix", "nu", "安装用户名前缀")
	fs.StringVar(&parsed.BasePath, "base-path", "/data/goldendb", "安装根目录")
	fs.StringVar(&parsed.InsUserPwd, "ins-user-pwd", "", "业务用户密码明文")
	fs.StringVar(&parsed.InsUserPwdB64, "ins-user-pwd-base64", "", "业务用户密码 base64")
	fs.IntVar(&parsed.HAMode, "ha-mode", 0, "高可用模式")
	fs.IntVar(&parsed.InstanceType, "instance-type", 1, "实例类型")
	fs.StringVar(&parsed.Charset, "charset", "utf8mb4", "字符集")
	fs.IntVar(&parsed.Mode, "mode", 1, "安装模式")
	fs.IntVar(&parsed.GTMUseMode, "gtm-use-mode", 1, "GTM 使用模式")
	fs.StringVar(&parsed.ClusterDesc, "cluster-desc", "", "集群描述")
	fs.BoolVar(&parsed.WaitCompletion, "wait-completion", false, "提交后等待任务完成")
	fs.IntVar(&parsed.MaxWaitTime, "max-wait-time", 3600, "最大等待秒数")
	fs.IntVar(&parsed.PollInterval, "poll-interval", 10, "轮询间隔秒数")
	fs.IntVar(&parsed.MaxRetries, "max-retries", 1, "失败重试次数")
	fs.BoolVar(&parsed.NoVerify, "no-verify", true, "跳过 SSL 证书校验")
	fs.BoolVar(&parsed.DryRun, "dry-run", false, "只渲染请求体")
	fs.StringVar(&parsed.Output, "output", "", "写入 JSON 文件")
	fs.StringVar(&parsed.Format, "format", "json", "输出格式")
	insightopen.AddAuthFlags(fs, &parsed.Auth)

	verifySSL := false
	fs.BoolVar(&verifySSL, "verify-ssl", false, "启用 SSL 证书校验")

	if err := fs.Parse(args); err != nil {
		return parsed, err
	}
	if verifySSL {
		parsed.NoVerify = false
	}
	if strings.TrimSpace(parsed.API) == "" {
		return parsed, fmt.Errorf("--api is required")
	}
	if strings.TrimSpace(parsed.CSV) == "" {
		return parsed, fmt.Errorf("--csv is required")
	}
	return parsed, nil
}

func resolvePasswordB64(password, passwordB64 string) (string, error) {
	if strings.TrimSpace(passwordB64) != "" {
		return strings.TrimSpace(passwordB64), nil
	}
	if password == "" {
		return "", fmt.Errorf("必须提供 --ins-user-pwd 或 --ins-user-pwd-base64")
	}
	return base64.StdEncoding.EncodeToString([]byte(password)), nil
}

func loadRows(path string) ([]normalizedRow, error) {
	rawRows, err := insightinput.ParseCSVInput(path)
	if err != nil {
		return nil, err
	}
	if len(rawRows) == 0 {
		return nil, fmt.Errorf("CSV 文件为空")
	}

	headers := map[string]struct{}{}
	for _, row := range rawRows {
		for key := range row {
			headers[key] = struct{}{}
		}
	}
	for _, header := range requiredHeaders {
		if _, ok := headers[header]; !ok {
			return nil, fmt.Errorf("CSV 缺少必填列: %s", header)
		}
	}

	rows := make([]normalizedRow, 0, len(rawRows))
	clusterNames := map[string]struct{}{}
	for i, row := range rawRows {
		clusterName := strings.TrimSpace(row["cluster_name"])
		if clusterName == "" {
			return nil, fmt.Errorf("第 %d 行 cluster_name 不能为空", i+1)
		}
		if _, ok := clusterNames[clusterName]; ok {
			return nil, fmt.Errorf("第 %d 行 cluster_name 重复: %s", i+1, clusterName)
		}
		clusterNames[clusterName] = struct{}{}

		roleIPs := map[string]string{}
		for _, role := range roleSequence {
			roleIPs[role] = strings.TrimSpace(row[role])
		}
		if roleIPs["M"] == "" || roleIPs["S"] == "" {
			return nil, fmt.Errorf("第 %d 行 M/S 不能为空", i+1)
		}

		seenIPs := map[string]struct{}{}
		for _, role := range roleSequence {
			ip := roleIPs[role]
			if ip == "" {
				continue
			}
			if _, ok := seenIPs[ip]; ok {
				return nil, fmt.Errorf("第 %d 行 IP 重复: %s", i+1, ip)
			}
			seenIPs[ip] = struct{}{}
		}

		serverType := strings.TrimSpace(row["server_type"])
		if serverType == "" {
			return nil, fmt.Errorf("第 %d 行 server_type 不能为空", i+1)
		}
		templates, err := resolveTemplates(serverType)
		if err != nil {
			return nil, err
		}

		rows = append(rows, normalizedRow{
			RowNo:            i + 1,
			Num:              strings.TrimSpace(row["num"]),
			ClusterName:      clusterName,
			ClusterGroupName: strings.TrimSpace(row["cluster_group_name"]),
			RoleIPs:          roleIPs,
			ServerType:       serverType,
			Templates:        templates,
		})
	}
	return rows, nil
}

func resolveTemplates(serverType string) (templateSelection, error) {
	normalized := strings.TrimSpace(serverType)
	if _, ok := supportedServerTypes[normalized]; !ok {
		return templateSelection{}, fmt.Errorf("不支持的 server_type: %s，当前仅支持 pm, vm_h, vm_l, vm_lowercase_0, vm_m", normalized)
	}
	return templateSelection{
		ServerType:     normalized,
		GlobalTemplate: fmt.Sprintf("template_%s_cluster.json", normalized),
		DNTemplate:     fmt.Sprintf("template_%s_dn.json", normalized),
		CNTemplate:     fmt.Sprintf("template_%s_cn.json", normalized),
	}, nil
}

func buildPayload(row normalizedRow, args runArgs, passwordB64 string) map[string]any {
	clusterDesc := strings.TrimSpace(args.ClusterDesc)
	if clusterDesc == "" {
		clusterDesc = row.ClusterName
	}
	return map[string]any{
		"configMode": 1,
		"clusterInstallInfo": map[string]any{
			"mode":         args.Mode,
			"charset":      args.Charset,
			"dbgroupNum":   1,
			"insUserPwd":   passwordB64,
			"clusterDesc":  clusterDesc,
			"clusterName":  row.ClusterName,
			"instanceType": args.InstanceType,
			"gtmUseMode":   args.GTMUseMode,
			"haMode":       args.HAMode,
		},
		"dnInstallList": buildDNInstallList(row, args),
		"cnInstallList": buildCNInstallList(row, args),
		"parameterTemplateInfos": []map[string]any{
			{"type": "DN", "templateName": row.Templates.DNTemplate},
			{"type": "CN", "templateName": row.Templates.CNTemplate},
			{"type": "GLOBAL", "templateName": row.Templates.GlobalTemplate},
		},
	}
}

func buildCNInstallList(row normalizedRow, args runArgs) []map[string]any {
	items := []map[string]any{}
	for _, role := range roleSequence {
		ip := row.RoleIPs[role]
		if ip == "" {
			continue
		}
		for _, item := range []struct {
			Suffix      int
			ServicePort int
		}{{1, 3306}, {2, 3307}} {
			installUser := fmt.Sprintf("%sdbproxy%d", args.Prefix, item.Suffix)
			items = append(items, map[string]any{
				"ip":          ip,
				"installPath": fmt.Sprintf("%s/%s", args.BasePath, installUser),
				"installUser": installUser,
				"servicePort": item.ServicePort,
			})
		}
	}
	return items
}

func buildDNInstallList(row normalizedRow, args runArgs) []map[string]any {
	installUser := fmt.Sprintf("%sdb1", args.Prefix)
	installPath := fmt.Sprintf("%s/%s", args.BasePath, installUser)
	dataPath := installPath + "/data"

	teamList := []map[string]any{}
	for _, role := range roleSequence {
		ip := row.RoleIPs[role]
		if ip == "" {
			continue
		}
		teamList = append(teamList, map[string]any{
			"teamId": roleToTeamID[role],
			"dnList": []map[string]any{{
				"ip":          ip,
				"dbRole":      roleToDBRole[role],
				"installPath": installPath,
				"installUser": installUser,
				"dataPath":    dataPath,
			}},
		})
	}
	return []map[string]any{{
		"dbgroupId": 1,
		"teamList":  teamList,
	}}
}

func executeRow(ctx context.Context, client *insightopen.Client, row normalizedRow, payload map[string]any, args runArgs) map[string]any {
	lastError := ""
	taskID := ""
	retries := args.MaxRetries
	if retries < 1 {
		retries = 1
	}

	for attempt := 1; attempt <= retries; attempt++ {
		task, err := startCreateCluster(ctx, client, payload)
		if err == nil {
			taskID = task
			var result any
			if args.WaitCompletion && taskID != "" {
				result, err = pollCreateClusterProgress(ctx, client, taskID, time.Duration(args.PollInterval)*time.Second, time.Duration(args.MaxWaitTime)*time.Second)
			}
			if err == nil {
				return map[string]any{
					"row_no":             row.RowNo,
					"num":                row.Num,
					"cluster_name":       row.ClusterName,
					"cluster_group_name": row.ClusterGroupName,
					"server_type":        row.ServerType,
					"template_selection": buildTemplateSelectionOutput(row),
					"status":             "success",
					"task_id":            taskID,
					"attempt":            attempt,
					"request_payload":    payload,
					"response":           result,
					"error":              "",
				}
			}
		}
		lastError = err.Error()
		log.Printf("集群 %s 第 %d/%d 次执行失败: %s", row.ClusterName, attempt, retries, err)
	}

	return map[string]any{
		"row_no":             row.RowNo,
		"num":                row.Num,
		"cluster_name":       row.ClusterName,
		"cluster_group_name": row.ClusterGroupName,
		"server_type":        row.ServerType,
		"template_selection": buildTemplateSelectionOutput(row),
		"status":             "failed",
		"task_id":            taskID,
		"attempt":            retries,
		"request_payload":    payload,
		"response":           nil,
		"error":              lastError,
	}
}

func startCreateCluster(ctx context.Context, client *insightopen.Client, payload map[string]any) (string, error) {
	var resp map[string]any
	if err := client.PostJSON(ctx, "/open_api/insight/external/tenant/createCluster", payload, &resp); err != nil {
		return "", err
	}
	if toInt(resp["code"]) != 1 {
		data, _ := json.Marshal(resp)
		return "", fmt.Errorf(string(data))
	}
	data, ok := resp["data"].(map[string]any)
	if ok {
		for _, key := range []string{"task_id", "taskId", "id"} {
			value := strings.TrimSpace(fmt.Sprint(data[key]))
			if value != "" && value != "<nil>" {
				return value, nil
			}
		}
	}
	dataText, _ := json.Marshal(resp)
	return "", fmt.Errorf("接口未返回 task_id: %s", string(dataText))
}

func pollCreateClusterProgress(ctx context.Context, client *insightopen.Client, taskID string, pollInterval, pollTimeout time.Duration) (map[string]any, error) {
	deadline := time.Now().Add(pollTimeout)
	var lastData map[string]any

	for time.Now().Before(deadline) {
		var resp insightopen.APIResponse
		path := fmt.Sprintf("/open_api/insight/external/tenant/getInstallClusterProcess?taskId=%s", taskID)
		if err := client.GetJSON(ctx, path, nil, &resp); err != nil {
			return nil, err
		}
		if resp.Code != 1 {
			return nil, fmt.Errorf(firstNonEmpty(resp.Msg))
		}

		data, err := insightopen.DecodeData[map[string]any](resp)
		if err != nil {
			return nil, fmt.Errorf("查询安装进度返回格式异常: %w", err)
		}
		lastData = data

		result := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["result"])))
		process := data["process"]
		log.Printf("taskId=%s progress=%v result=%s", taskID, process, firstNonEmpty(result, "unknown"))

		if result == "success" {
			return data, nil
		}
		if result == "fail" {
			return nil, fmt.Errorf("taskId=%s errorCode=%v errorMsg=%s", taskID, data["errorCode"], firstNonEmpty(fmt.Sprint(data["errorMsg"]), "install failed"))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return nil, fmt.Errorf("任务轮询超时: taskId=%s, last_process=%v, last_result=%v", taskID, lastData["process"], lastData["result"])
}

func buildTemplateSelectionOutput(row normalizedRow) map[string]any {
	return map[string]any{
		"server_type":     row.Templates.ServerType,
		"global_template": row.Templates.GlobalTemplate,
		"dn_template":     row.Templates.DNTemplate,
		"cn_template":     row.Templates.CNTemplate,
	}
}

func logTemplateSelection(rows []normalizedRow) {
	for _, row := range rows {
		text, _ := json.Marshal(buildTemplateSelectionOutput(row))
		log.Printf("cluster=%s template_selection=%s", row.ClusterName, string(text))
	}
}

func writeOutput(path string, payload map[string]any) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func renderStdout(format string, output map[string]any) {
	if format == "json" {
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return
	}

	summary, _ := output["summary"].(map[string]any)
	log.Printf("总计 total=%v success=%v failed=%v", summary["total"], summary["success_count"], summary["failed_count"])
	items, _ := output["clusters"].([]map[string]any)
	if items == nil {
		if generic, ok := output["clusters"].([]any); ok {
			for _, item := range generic {
				cluster, _ := item.(map[string]any)
				log.Printf(
					"cluster=%v server_type=%v status=%v task_id=%v templates=%v error=%v",
					cluster["cluster_name"],
					cluster["server_type"],
					cluster["status"],
					cluster["task_id"],
					cluster["template_selection"],
					cluster["error"],
				)
			}
		}
	}
}

func renderTopLevelError(err error) error {
	output := map[string]any{
		"success": false,
		"summary": map[string]any{"total": 0, "success_count": 0, "failed_count": 0},
		"error":   err.Error(),
	}
	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
	return nil
}

func toInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		var out int
		fmt.Sscanf(fmt.Sprint(value), "%d", &out)
		return out
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
