package insightbatchdn

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

	"github.com/ywhywl/gdbtools/internal/insightinput"
	"github.com/ywhywl/gdbtools/internal/insightopen"
)

type args struct {
	Input              string
	DefaultPort        string
	DefaultAdminPort   string
	DefaultInstallUser string
	DefaultInstallPath string
	DefaultDataPath    string
	DefaultLogPath     string
	PollInterval       int
	PollTimeout        int
	VerifySSL          bool
	OutputJSON         bool
}

type dnRow struct {
	RowNo                int
	APIBase              string
	ClusterName          string
	TemplateName         string
	DBGroupName          string
	DBGroupID            string
	TeamID               string
	IP                   string
	Port                 string
	BackupSelectStrategy string
	BackupStartTime      string
	BackupEndTime        string
	BackupID             string
	AdminPort            string
	InstallUser          string
	InstallPath          string
	DataPath             string
	LogPath              string
}

type dnGroup struct {
	APIBase      string
	ClusterName  string
	TemplateName string
	Rows         []dnRow
}

func Run(argv []string) (int, error) {
	fs := flag.NewFlagSet("insight-batch-add-dn", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var parsed args
	fs.StringVar(&parsed.Input, "input", "", "输入文件路径 (CSV 或 JSON)")
	fs.StringVar(&parsed.DefaultPort, "default-port", "", "默认 DN 端口")
	fs.StringVar(&parsed.DefaultAdminPort, "default-admin-port", "", "默认 DN 管理端口")
	fs.StringVar(&parsed.DefaultInstallUser, "default-install-user", "", "默认安装用户")
	fs.StringVar(&parsed.DefaultInstallPath, "default-install-path", "", "默认安装路径")
	fs.StringVar(&parsed.DefaultDataPath, "default-data-path", "", "默认数据路径")
	fs.StringVar(&parsed.DefaultLogPath, "default-log-path", "", "默认日志路径")
	fs.IntVar(&parsed.PollInterval, "poll-interval", 10, "轮询间隔")
	fs.IntVar(&parsed.PollTimeout, "poll-timeout", 3600, "轮询超时")
	fs.BoolVar(&parsed.VerifySSL, "verify-ssl", false, "启用 SSL 证书校验；默认关闭")
	fs.BoolVar(&parsed.OutputJSON, "output-json", false, "输出 JSON")
	if err := fs.Parse(argv); err != nil {
		return 2, err
	}
	if strings.TrimSpace(parsed.Input) == "" {
		return 2, fmt.Errorf("--input is required")
	}

	rows, err := insightinput.ParseTabularInput(parsed.Input)
	if err != nil {
		return 2, err
	}
	normalized, err := normalizeDNRows(rows, parsed)
	if err != nil {
		return 2, err
	}
	grouped := groupDNRows(normalized)

	groupResults := make([]map[string]any, 0, len(grouped))
	itemResults := make([]map[string]any, 0, len(normalized))

	for _, group := range grouped {
		client, err := insightopen.NewClient(group.APIBase, !parsed.VerifySSL)
		if err != nil {
			return 2, err
		}
		clusterID, err := insightopen.SearchClusterID(context.Background(), client, group.ClusterName)
		if err != nil {
			return 2, err
		}
		payloadRows := cloneDNRows(group.Rows)
		payload, err := buildDNPayload(context.Background(), client, clusterID, group.TemplateName, payloadRows)
		if err != nil {
			return 2, err
		}
		taskID, err := insightopen.StartInstallTask(context.Background(), client, "/open_api/insight/external/install/batchAddSlaveDN", payload)
		if err != nil {
			return 2, err
		}
		finalData, err := insightopen.PollTaskResult(context.Background(), client, "/open_api/insight/external/install/querybatchAddSlaveDNResult", taskID, insightopen.PollOptions{
			Interval: time.Duration(parsed.PollInterval) * time.Second,
			Timeout:  time.Duration(parsed.PollTimeout) * time.Second,
		})
		if err != nil {
			return 2, err
		}

		records := toRecords(finalData["records"])
		groupItems := summarizeDNGroup(records, payloadRows)
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

func normalizeDNRows(rows []map[string]string, args args) ([]dnRow, error) {
	out := make([]dnRow, 0, len(rows))
	for i, row := range rows {
		api := strings.TrimSpace(row["insight_addr"])
		cluster := strings.TrimSpace(row["cluster_name"])
		template := strings.TrimSpace(row["template_name"])
		ip := strings.TrimSpace(row["ip"])
		dbgroupName := strings.TrimSpace(row["dbgroup_name"])
		dbgroupID := strings.TrimSpace(row["dbgroup_id"])
		if api == "" || cluster == "" || template == "" || ip == "" {
			return nil, fmt.Errorf("第 %d 行缺少必填字段 insight_addr/cluster_name/template_name/ip", i+1)
		}
		if dbgroupName == "" && dbgroupID == "" {
			return nil, fmt.Errorf("第 %d 行缺少 dbgroup_name 或 dbgroup_id", i+1)
		}
		apiBase, err := insightopen.NormalizeAPIBase(api)
		if err != nil {
			return nil, err
		}
		out = append(out, dnRow{
			RowNo:                i + 1,
			APIBase:              apiBase,
			ClusterName:          cluster,
			TemplateName:         template,
			DBGroupName:          dbgroupName,
			DBGroupID:            dbgroupID,
			TeamID:               strings.TrimSpace(row["team_id"]),
			IP:                   ip,
			Port:                 firstNonEmpty(strings.TrimSpace(row["port"]), args.DefaultPort),
			BackupSelectStrategy: strings.TrimSpace(row["backup_select_strategy"]),
			BackupStartTime:      strings.TrimSpace(row["backup_start_time"]),
			BackupEndTime:        strings.TrimSpace(row["backup_end_time"]),
			BackupID:             strings.TrimSpace(row["backup_id"]),
			AdminPort:            firstNonEmpty(strings.TrimSpace(row["admin_port"]), args.DefaultAdminPort),
			InstallUser:          firstNonEmpty(strings.TrimSpace(row["install_user"]), args.DefaultInstallUser),
			InstallPath:          firstNonEmpty(strings.TrimSpace(row["install_path"]), args.DefaultInstallPath),
			DataPath:             firstNonEmpty(strings.TrimSpace(row["data_path"]), args.DefaultDataPath),
			LogPath:              firstNonEmpty(strings.TrimSpace(row["log_path"]), args.DefaultLogPath),
		})
	}
	return out, nil
}

func groupDNRows(rows []dnRow) []dnGroup {
	index := map[string]int{}
	out := make([]dnGroup, 0)
	for _, row := range rows {
		key := row.APIBase + "\x00" + row.ClusterName + "\x00" + row.TemplateName
		if pos, ok := index[key]; ok {
			out[pos].Rows = append(out[pos].Rows, row)
			continue
		}
		index[key] = len(out)
		out = append(out, dnGroup{APIBase: row.APIBase, ClusterName: row.ClusterName, TemplateName: row.TemplateName, Rows: []dnRow{row}})
	}
	return out
}

func cloneDNRows(rows []dnRow) []dnRow {
	out := make([]dnRow, len(rows))
	copy(out, rows)
	return out
}

func buildDNPayload(ctx context.Context, client *insightopen.Client, clusterID int, templateName string, rows []dnRow) (map[string]any, error) {
	type teamBucket struct {
		TeamID string
		Backup string
		Rows   []dnRow
	}
	dbgroupMap := map[int]map[string]*teamBucket{}

	for i := range rows {
		dbgroupID := 0
		if rows[i].DBGroupID != "" {
			dbgroupID = mustInt(rows[i].DBGroupID)
		} else {
			resolvedID, err := insightopen.ResolveDBGroupID(ctx, client, clusterID, rows[i].DBGroupName)
			if err != nil {
				return nil, err
			}
			dbgroupID = resolvedID
			rows[i].DBGroupID = strconv.Itoa(resolvedID)
		}
		if _, ok := dbgroupMap[dbgroupID]; !ok {
			dbgroupMap[dbgroupID] = map[string]*teamBucket{}
		}
		backupKeyBytes, _ := json.Marshal(map[string]string{
			"backup_select_strategy": rows[i].BackupSelectStrategy,
			"backup_start_time":      rows[i].BackupStartTime,
			"backup_end_time":        rows[i].BackupEndTime,
			"backup_id":              rows[i].BackupID,
		})
		backupKey := rows[i].TeamID + "\x00" + string(backupKeyBytes)
		if _, ok := dbgroupMap[dbgroupID][backupKey]; !ok {
			dbgroupMap[dbgroupID][backupKey] = &teamBucket{TeamID: rows[i].TeamID, Backup: string(backupKeyBytes)}
		}
		dbgroupMap[dbgroupID][backupKey].Rows = append(dbgroupMap[dbgroupID][backupKey].Rows, rows[i])
	}

	dbgroupList := make([]map[string]any, 0, len(dbgroupMap))
	for dbgroupID, teamGroups := range dbgroupMap {
		teamList := make([]map[string]any, 0, len(teamGroups))
		for _, bucket := range teamGroups {
			teamItem := map[string]any{"dnList": []map[string]any{}}
			if strings.TrimSpace(bucket.TeamID) != "" {
				teamItem["teamId"] = mustInt(bucket.TeamID)
			}
			var backupRaw map[string]string
			_ = json.Unmarshal([]byte(bucket.Backup), &backupRaw)
			if strings.TrimSpace(backupRaw["backup_select_strategy"]) != "" {
				backupTask := map[string]any{
					"selectStrategy": mustInt(backupRaw["backup_select_strategy"]),
				}
				if backupRaw["backup_start_time"] != "" {
					backupTask["startTime"] = backupRaw["backup_start_time"]
				}
				if backupRaw["backup_end_time"] != "" {
					backupTask["endTime"] = backupRaw["backup_end_time"]
				}
				if backupRaw["backup_id"] != "" {
					backupTask["backupId"] = backupRaw["backup_id"]
				}
				teamItem["backupTask"] = backupTask
			}
			dnList := make([]map[string]any, 0, len(bucket.Rows))
			for _, row := range bucket.Rows {
				dnItem := map[string]any{"ip": row.IP}
				if row.Port != "" {
					dnItem["port"] = mustInt(row.Port)
				}
				if row.AdminPort != "" {
					dnItem["adminPort"] = mustInt(row.AdminPort)
				}
				if row.InstallUser != "" {
					dnItem["installUser"] = row.InstallUser
				}
				if row.InstallPath != "" {
					dnItem["installPath"] = row.InstallPath
				}
				if row.DataPath != "" {
					dnItem["dataPath"] = row.DataPath
				}
				if row.LogPath != "" {
					dnItem["logPath"] = row.LogPath
				}
				dnList = append(dnList, dnItem)
			}
			teamItem["dnList"] = dnList
			teamList = append(teamList, teamItem)
		}
		dbgroupList = append(dbgroupList, map[string]any{"dbgroupId": dbgroupID, "teamList": teamList})
	}

	return map[string]any{
		"clusterId": clusterID,
		"parameterTemplateInfos": []map[string]any{
			{"type": "DN", "templateName": templateName},
		},
		"dbgroupList": dbgroupList,
	}, nil
}

func summarizeDNGroup(records []map[string]any, rows []dnRow) []map[string]any {
	recordMap := map[string]map[string]any{}
	for _, item := range records {
		key := toString(item["dbgroupId"]) + "\x00" + toString(item["teamId"]) + "\x00" + toString(item["ip"]) + "\x00" + toString(item["port"])
		recordMap[key] = item
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		key := row.DBGroupID + "\x00" + row.TeamID + "\x00" + row.IP + "\x00" + row.Port
		matched := recordMap[key]
		result := "success"
		message := ""
		dbgroupID := row.DBGroupID
		teamID := row.TeamID
		port := row.Port
		if matched != nil {
			if toString(matched["result"]) != "" {
				result = toString(matched["result"])
			}
			message = toString(matched["failMsg"])
			dbgroupID = firstNonEmpty(toString(matched["dbgroupId"]), dbgroupID)
			teamID = firstNonEmpty(toString(matched["teamId"]), teamID)
			port = firstNonEmpty(toString(matched["port"]), port)
		}
		status := "success"
		if result != "success" {
			status = "failed"
		}
		out = append(out, map[string]any{
			"row_no":     row.RowNo,
			"dbgroup_id": dbgroupID,
			"team_id":    teamID,
			"ip":         row.IP,
			"port":       port,
			"status":     status,
			"message":    message,
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
