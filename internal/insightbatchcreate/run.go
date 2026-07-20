package insightbatchcreate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ywhywl/gdbtools/internal/hostchecker"
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
	requiredHeadersAlways = []string{"num", "cluster_name", "cluster_group_name", "M", "S", "TS", "LS", "OS"}
	requiredHeaders       = []string{"num", "cluster_name", "cluster_group_name", "M", "S", "TS", "LS", "OS", "server_type"}
	roleSequence          = []string{"M", "S", "LS", "OS", "TS"}
	roleToTeamID          = map[string]int{"M": 1, "S": 2, "LS": 3, "OS": 4, "TS": 5}
	roleToDBRole          = map[string]int{"M": 1, "S": 0, "TS": 0, "LS": 0, "OS": 2}
)

type templateSelection struct {
	ServerType      string `json:"server_type"`
	GlobalTemplate  string `json:"global_template"`
	DNTemplate      string `json:"dn_template"`
	CNTemplate      string `json:"cn_template"`
	ClusterTemplate string `json:"cluster_template,omitempty"`
	GTMTemplate     string `json:"gtm_template,omitempty"`
	LDSTemplate     string `json:"lds_template,omitempty"`
	SystemTemplate  string `json:"system_template,omitempty"`
	DnOSTemplate    string `json:"dn_os_template,omitempty"`
}

type normalizedRow struct {
	RowNo            int
	Num              string
	ClusterName      string
	ClusterGroupName string
	RoleIPs          map[string]string
	ServerType       string
	CSVServerType    string // CSV-specified value, "" when auto-selected
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
	// Template auto-selection
	AutoSelect     bool
	SSHUser        string
	SSHKey         string
	SSHPassword    string
	SSHPasswordB64 string
	SSHPort        int
	SSHTimeout     int
	CaseSensitive  bool
	IgnoreMismatch bool
	SkipCheck      bool
	AllowLowMemVM  bool
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
		return 3, renderTopLevelError(err)
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

	rows, err := loadRows(parsedArgs.CSV, parsedArgs.AutoSelect)
	if err != nil {
		return 3, renderTopLevelError(err)
	}

	// Phase 2: SSH-based template auto-selection
	if parsedArgs.AutoSelect {
		rows, err = applyAutoSelect(rows, parsedArgs)
		if err != nil {
			return 3, renderTopLevelError(err)
		}
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

	// Template auto-selection flags
	fs.BoolVar(&parsed.AutoSelect, "auto-select-template", true, "通过 SSH 检测内存自动选择模版（默认开启）")
	fs.StringVar(&parsed.SSHUser, "ssh-user", "", "SSH 用户名（自动选择模版时需要）")
	fs.StringVar(&parsed.SSHKey, "ssh-key", "", "SSH 私钥路径（不指定则自动查找 ~/.ssh/id_*）")
	fs.StringVar(&parsed.SSHPassword, "ssh-password", "", "SSH 密码明文（key 认证失败时的兜底）")
	fs.StringVar(&parsed.SSHPasswordB64, "ssh-password-b64", "", "SSH 密码 base64")
	fs.IntVar(&parsed.SSHPort, "ssh-port", 22, "SSH 端口")
	fs.IntVar(&parsed.SSHTimeout, "ssh-timeout", 15, "单台 SSH 超时秒数")
	fs.BoolVar(&parsed.CaseSensitive, "case-sensitive", false, "数据库表名大小写敏感，开启后 DN 模版名附加 _lowercase_0 后缀")
	fs.BoolVar(&parsed.IgnoreMismatch, "ignore-template-mismatch", false, "自动选择与 CSV 指定不一致时，使用自动选择的结果")
	fs.BoolVar(&parsed.SkipCheck, "skip-template-check", false, "自动选择与 CSV 指定不一致时，使用 CSV 指定值继续执行")
	fs.BoolVar(&parsed.AllowLowMemVM, "allow-low-memory-vm", false, "允许虚拟机内存低于24G时不报错，降级使用 vm_l 模版")
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
	if parsed.AutoSelect {
		if strings.TrimSpace(parsed.SSHUser) == "" {
			return parsed, fmt.Errorf("开启 --auto-select-template 时，--ssh-user 为必填项")
		}
	}
	if parsed.IgnoreMismatch && parsed.SkipCheck {
		return parsed, fmt.Errorf("--ignore-template-mismatch 和 --skip-template-check 不能同时指定")
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

// loadRows parses the CSV and validates headers. When autoSelect is true,
// server_type is optional; otherwise it is required.
func loadRows(path string, autoSelect bool) ([]normalizedRow, error) {
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
	req := requiredHeadersAlways
	if !autoSelect {
		req = requiredHeaders
	}
	for _, header := range req {
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
		csvServerType := serverType

		if serverType == "" && !autoSelect {
			return nil, fmt.Errorf("第 %d 行 server_type 不能为空", i+1)
		}

		templates := templateSelection{}
		if serverType != "" {
			t, err := resolveTemplates(serverType, false)
			if err != nil {
				return nil, err
			}
			templates = t
		}

		rows = append(rows, normalizedRow{
			RowNo:            i + 1,
			Num:              strings.TrimSpace(row["num"]),
			ClusterName:      clusterName,
			ClusterGroupName: strings.TrimSpace(row["cluster_group_name"]),
			RoleIPs:          roleIPs,
			ServerType:       serverType,
			CSVServerType:    csvServerType,
			Templates:        templates,
		})
	}
	return rows, nil
}

// applyAutoSelect performs SSH-based template detection for each cluster,
// validates against CSV values, and updates rows accordingly.
func applyAutoSelect(rows []normalizedRow, args runArgs) ([]normalizedRow, error) {
	sshAuth, err := hostchecker.NewSSHAuth(args.SSHKey, args.SSHPassword, args.SSHPasswordB64)
	if err != nil {
		return nil, err
	}
	if !sshAuth.HasAuth() {
		return nil, fmt.Errorf("模版自动选择需要 SSH 认证: 请配置 SSH key、SSH agent 或提供 --ssh-password")
	}
	if sshAuth.KeyPath != "" {
		log.Printf("[模版检测] SSH 认证: key=%s", sshAuth.KeyPath)
	} else if sshAuth.AgentSock != "" {
		log.Printf("[模版检测] SSH 认证: agent=%s", sshAuth.AgentSock)
	}

	// Collect unique IPs per cluster (only non-empty IPs).
	clusterIPs := make(map[string][]string)
	clusterRows := make(map[string][]int) // clusterName -> row indices
	for i, row := range rows {
		var ips []string
		for _, role := range roleSequence {
			ip := row.RoleIPs[role]
			if ip != "" {
				ips = append(ips, ip)
			}
		}
		clusterIPs[row.ClusterName] = ips
		clusterRows[row.ClusterName] = append(clusterRows[row.ClusterName], i)
	}

	// SSH detect server_type per cluster.
	clusterDetected := make(map[string]string)
	sshTimeout := time.Duration(args.SSHTimeout) * time.Second
	for _, name := range sortedKeys(clusterIPs) {
		ips := clusterIPs[name]
		if len(ips) == 0 {
			return nil, fmt.Errorf("集群 %s 无任何有效 IP 地址", name)
		}
		log.Printf("[模版检测] 集群 %s: 正在通过 SSH 检测 %d 台主机...", name, len(ips))
		st, err := resolveClusterServerType(ips, args.SSHPort, args.SSHUser, sshAuth, sshTimeout, args.AllowLowMemVM)
		if err != nil {
			return nil, fmt.Errorf("集群 %s 模版检测失败: %w", name, err)
		}
		clusterDetected[name] = st
		log.Printf("[模版检测] 集群 %s: 检测到 server_type=%s", name, st)
	}

	// Apply detected types and handle CSV mismatches.
	for i, row := range rows {
		detected := clusterDetected[row.ClusterName]
		csvVal := row.CSVServerType

		if csvVal == "" || detected == csvVal {
			// No conflict: update row with detected type.
			templates, err := resolveTemplates(detected, args.CaseSensitive)
			if err != nil {
				return nil, err
			}
			rows[i].ServerType = detected
			rows[i].Templates = templates
			continue
		}

		// Conflict: CSV value differs from detected.
		if args.IgnoreMismatch {
			log.Printf("[模版检测] 集群 %s: CSV 指定 %s，自动检测 %s — 使用自动选择结果 (--ignore-template-mismatch)", row.ClusterName, csvVal, detected)
			templates, err := resolveTemplates(detected, args.CaseSensitive)
			if err != nil {
				return nil, err
			}
			rows[i].ServerType = detected
			rows[i].Templates = templates
		} else if args.SkipCheck {
			log.Printf("[模版检测] 集群 %s: CSV 指定 %s，自动检测 %s — 使用 CSV 指定值 (--skip-template-check)", row.ClusterName, csvVal, detected)
			// Keep CSV value, re-resolve templates with caseSensitive.
			templates, err := resolveTemplates(csvVal, args.CaseSensitive)
			if err != nil {
				return nil, err
			}
			rows[i].ServerType = csvVal
			rows[i].Templates = templates
		} else {
			return nil, fmt.Errorf("集群 %s: CSV 指定 server_type=%s，自动检测为 %s。请检查服务器配置，或使用 --ignore-template-mismatch（采用自动检测结果）/ --skip-template-check（采用 CSV 值）继续", row.ClusterName, csvVal, detected)
		}
	}

	return rows, nil
}

// resolveTemplates builds a templateSelection from server_type.
// When caseSensitive is true, _lowercase_0 is appended to the server_type part of ALL template names.
func resolveTemplates(serverType string, caseSensitive bool) (templateSelection, error) {
	normalized := strings.TrimSpace(serverType)
	if normalized == "" {
		return templateSelection{}, fmt.Errorf("server_type 为空")
	}
	if _, ok := supportedServerTypes[normalized]; !ok {
		return templateSelection{}, fmt.Errorf("不支持的 server_type: %s，当前仅支持 pm, vm_h, vm_l, vm_lowercase_0, vm_m", normalized)
	}

	suffix := ""
	if caseSensitive {
		suffix = "_lowercase_0"
	}

	return templateSelection{
		ServerType:      normalized,
		GlobalTemplate:  fmt.Sprintf("template_%s_cluster.json", normalized+suffix),
		DNTemplate:      fmt.Sprintf("template_%s_dn.json", normalized+suffix),
		CNTemplate:      fmt.Sprintf("template_%s_cn.json", normalized+suffix),
		ClusterTemplate: fmt.Sprintf("template_%s_cluster.json", normalized+suffix),
		GTMTemplate:     fmt.Sprintf("template_%s_gtm.json", normalized+suffix),
		LDSTemplate:     fmt.Sprintf("template_%s_lds.json", normalized+suffix),
		SystemTemplate:  fmt.Sprintf("template_%s_system.json", normalized+suffix),
		DnOSTemplate:    fmt.Sprintf("template_%s_dn_OS.json", normalized+suffix),
	}, nil
}

func buildPayload(row normalizedRow, args runArgs, passwordB64 string) map[string]any {
	clusterDesc := strings.TrimSpace(args.ClusterDesc)
	if clusterDesc == "" {
		clusterDesc = row.ClusterName
	}

	templateInfos := []map[string]any{
		{"type": "DN", "templateName": row.Templates.DNTemplate},
		{"type": "CN", "templateName": row.Templates.CNTemplate},
		{"type": "GLOBAL", "templateName": row.Templates.GlobalTemplate},
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
		"dnInstallList":          buildDNInstallList(row, args),
		"cnInstallList":          buildCNInstallList(row, args),
		"parameterTemplateInfos": templateInfos,
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
		return "", fmt.Errorf("%s", string(data))
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
			return nil, fmt.Errorf("%s", firstNonEmpty(resp.Msg))
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
	out := map[string]any{
		"server_type":     row.Templates.ServerType,
		"global_template": row.Templates.GlobalTemplate,
		"dn_template":     row.Templates.DNTemplate,
		"cn_template":     row.Templates.CNTemplate,
	}
	if row.Templates.ClusterTemplate != "" {
		out["cluster_template"] = row.Templates.ClusterTemplate
	}
	if row.Templates.GTMTemplate != "" {
		out["gtm_template"] = row.Templates.GTMTemplate
	}
	if row.Templates.LDSTemplate != "" {
		out["lds_template"] = row.Templates.LDSTemplate
	}
	if row.Templates.SystemTemplate != "" {
		out["system_template"] = row.Templates.SystemTemplate
	}
	if row.Templates.DnOSTemplate != "" {
		out["dn_os_template"] = row.Templates.DnOSTemplate
	}
	return out
}

func logTemplateSelection(rows []normalizedRow) {
	// Summary table
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "集群\tserver_type\t来源\tDN 模版\tCN 模版\tGLOBAL 模版")
	for _, row := range rows {
		source := "自动"
		if row.CSVServerType != "" {
			source = "CSV"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			row.ClusterName,
			row.Templates.ServerType,
			source,
			row.Templates.DNTemplate,
			row.Templates.CNTemplate,
			row.Templates.GlobalTemplate,
		)
	}
	w.Flush()

	// Also log JSON for machine parsing
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

// sortedKeys returns sorted keys of a map[string]T.
func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
