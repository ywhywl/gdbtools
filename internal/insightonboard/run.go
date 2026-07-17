package insightonboard

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

	"github.com/ywhywl/gdbtools/internal/hostchecker"
	"github.com/ywhywl/gdbtools/internal/insightinput"
	"github.com/ywhywl/gdbtools/internal/insightopen"
)

const defaultSSHPasswordB64 = "c2VjcmV0"

type apiErrorData struct {
	ErrorCode any      `json:"errorCode"`
	ErrorMsg  string   `json:"errorMsg"`
	FailedIP  []string `json:"failedIp"`
	TaskID    any      `json:"taskId"`
}

type hostTaskResponse struct {
	Result        string   `json:"result"`
	InstallStatus any      `json:"installStatus"`
	ErrorCode     any      `json:"errorCode"`
	ErrorMsg      string   `json:"errorMsg"`
	FailedIP      []string `json:"failedIp"`
}

func Run(args []string) (int, error) {
	log.SetFlags(log.LstdFlags)

	fs := flag.NewFlagSet("insight-batch-onboard-hosts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var api string
	var input string
	var sshPort int
	var sshUser string
	var sshPassword string
	var sshPasswordB64 string
	var coverInstall int
	var batchSize int
	var pollInterval int
	var pollTimeout int
	var verifySSL bool
	var outputJSON bool
	var debug bool
	var skipCheck bool
	var checkTimeout int
	var authFlags insightopen.AuthFlags

	fs.StringVar(&api, "api", "", "Insight API 地址")
	fs.StringVar(&input, "input", "", "输入文件路径 (CSV 或 JSON)")
	fs.IntVar(&sshPort, "ssh-port", 22, "SSH 端口")
	fs.StringVar(&sshUser, "ssh-user", "", "SSH 用户名")
	fs.StringVar(&sshPassword, "ssh-password", "", "SSH 密码明文")
	fs.StringVar(&sshPasswordB64, "ssh-password-b64", defaultSSHPasswordB64, "SSH 密码的 base64 串")
	fs.IntVar(&coverInstall, "cover-install", 0, "是否覆盖安装: 0=否,1=是")
	fs.IntVar(&batchSize, "batch-size", 10, "每批纳管主机数")
	fs.IntVar(&pollInterval, "poll-interval", 10, "轮询间隔(秒)")
	fs.IntVar(&pollTimeout, "poll-timeout", 3600, "轮询超时(秒)")
	fs.BoolVar(&verifySSL, "verify-ssl", false, "启用 SSL 证书校验")
	fs.BoolVar(&outputJSON, "output-json", false, "以 JSON 格式输出结果")
	fs.BoolVar(&debug, "debug", false, "打印请求和响应的 debug 日志")
	fs.BoolVar(&skipCheck, "skip-check", false, "跳过主机前置检查")
	fs.IntVar(&checkTimeout, "check-timeout", 15, "单台主机检查超时(秒)")
	insightopen.AddAuthFlags(fs, &authFlags)

	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if strings.TrimSpace(api) == "" {
		return 2, fmt.Errorf("--api is required")
	}
	if strings.TrimSpace(input) == "" {
		return 2, fmt.Errorf("--input is required")
	}
	if strings.TrimSpace(sshUser) == "" {
		return 2, fmt.Errorf("--ssh-user is required")
	}
	if batchSize < 1 || batchSize > 10 {
		return 1, fmt.Errorf("--batch-size 必须在 1 到 10 之间")
	}

	insightopen.SetDebug(debug)

	hosts, err := insightinput.ParseTabularInput(input)
	if err != nil {
		return 2, err
	}
	if len(hosts) == 0 {
		return 1, fmt.Errorf("输入文件中没有有效的服务器数据")
	}

	auth, err := insightopen.ResolveAuth(authFlags)
	if err != nil {
		return 2, err
	}

	client, err := insightopen.NewClient(api, !verifySSL, auth)
	if err != nil {
		return 2, err
	}
	log.Printf("待纳管主机: %d 台 (Insight: %s)", len(hosts), client.BaseURL())

	// --- Pre-check hosts ---
	var checkResults []hostchecker.CheckResult
	if skipCheck {
		log.Printf("跳过前置检查 (--skip-check)")
	} else {
		resolvedB64 := resolveSSHPasswordB64(sshPassword, sshPasswordB64)
		log.Printf("开始前置检查，超时 %ds/台", checkTimeout)
		checkResults = hostchecker.CheckAll(hosts, sshPort, sshUser, resolvedB64, time.Duration(checkTimeout)*time.Second)

		passed := 0
		failed := 0
		for _, r := range checkResults {
			if r.Passed {
				passed++
			} else {
				failed++
			}
		}
		log.Printf("前置检查完成: 通过 %d, 未通过 %d", passed, failed)

		// Filter: keep only passed hosts
		passedIPs := map[string]bool{}
		for _, r := range checkResults {
			if r.Passed {
				passedIPs[r.IP] = true
			}
		}
		filtered := make([]map[string]string, 0, passed)
		for _, host := range hosts {
			if passedIPs[strings.TrimSpace(host["server_ip"])] {
				filtered = append(filtered, host)
			}
		}
		hosts = filtered
		log.Printf("进入纳管的主机: %d 台", len(hosts))

		if len(hosts) == 0 {
			if outputJSON {
				output, _ := json.MarshalIndent(map[string]any{
					"total":         len(checkResults),
					"success_count": 0,
					"failed_count":  0,
					"precheck":      buildPrecheckOutput(checkResults),
					"results":       []map[string]string{},
				}, "", "  ")
				fmt.Println(string(output))
			}
			return 1, fmt.Errorf("所有主机均未通过前置检查")
		}
	}

	ipStatus, runErr := onboardHosts(
		context.Background(),
		client,
		hosts,
		sshPort,
		sshUser,
		sshPassword,
		sshPasswordB64,
		coverInstall,
		batchSize,
		time.Duration(pollInterval)*time.Second,
		time.Duration(pollTimeout)*time.Second,
	)
	if runErr != nil {
		log.Printf("纳管失败: %s", runErr)
		ipStatus = map[string]string{}
		for _, host := range hosts {
			ipStatus[strings.TrimSpace(host["server_ip"])] = "failed"
		}
	}

	successCount := 0
	results := make([]map[string]string, 0, len(ipStatus))
	for ip, status := range ipStatus {
		if status == "success" {
			successCount++
		}
		results = append(results, map[string]string{"ip": ip, "status": status})
	}
	failedCount := len(ipStatus) - successCount

	if outputJSON {
		out := map[string]any{
			"total":         len(ipStatus),
			"success_count": successCount,
			"failed_count":  failedCount,
			"results":       results,
		}
		if len(checkResults) > 0 {
			out["precheck"] = buildPrecheckOutput(checkResults)
		}
		output, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return 2, err
		}
		fmt.Println(string(output))
	} else {
		log.Printf("纳管完成: 总计 %d, 成功 %d, 失败 %d", len(ipStatus), successCount, failedCount)
		for ip, status := range ipStatus {
			mark := "FAIL"
			if status == "success" {
				mark = "OK"
			}
			log.Printf("  [%s] %s", mark, ip)
		}
	}

	if failedCount == 0 {
		return 0, nil
	}
	return 1, nil
}

func onboardHosts(
	ctx context.Context,
	client *insightopen.Client,
	hosts []map[string]string,
	sshPort int,
	sshUser string,
	sshPassword string,
	sshPasswordB64 string,
	coverInstall int,
	batchSize int,
	pollInterval time.Duration,
	pollTimeout time.Duration,
) (map[string]string, error) {
	resolvedPasswordB64 := resolveSSHPasswordB64(sshPassword, sshPasswordB64)
	batches, err := chunkHosts(hosts, batchSize)
	if err != nil {
		return nil, err
	}

	status := map[string]string{}
	for i, batch := range batches {
		ips := make([]string, 0, len(batch))
		for _, host := range batch {
			ips = append(ips, strings.TrimSpace(host["server_ip"]))
		}
		log.Printf("开始第 %d/%d 批纳管，本批 %d 台: %s", i+1, len(batches), len(batch), strings.Join(ips, ", "))

		if err := processBatch(ctx, client, batch, sshPort, sshUser, resolvedPasswordB64, coverInstall, pollInterval, pollTimeout, status); err != nil {
			log.Printf("第 %d/%d 批纳管失败: %s", i+1, len(batches), err)
			for _, host := range batch {
				status[strings.TrimSpace(host["server_ip"])] = "failed"
			}
		}
	}
	return status, nil
}

func processBatch(
	ctx context.Context,
	client *insightopen.Client,
	batch []map[string]string,
	sshPort int,
	sshUser string,
	sshPasswordB64 string,
	coverInstall int,
	pollInterval time.Duration,
	pollTimeout time.Duration,
	status map[string]string,
) error {
	payload := buildBatchPayload(batch, sshPort, sshUser, sshPasswordB64)

	var resp map[string]any
	path := fmt.Sprintf("/open_api/insight/external/host/batchAddhost?coverInstall=%d", coverInstall)
	if err := client.PostJSON(ctx, path, payload, &resp); err != nil {
		return err
	}
	code := toInt(resp["code"])
	if code != 1 {
		msg := fmt.Sprintf("纳管接口返回 code=%v", resp["code"])
		if data, ok := resp["data"].(map[string]any); ok {
			ed := apiErrorData{
				ErrorCode: data["errorCode"],
				ErrorMsg:  strings.TrimSpace(fmt.Sprint(data["errorMsg"])),
				FailedIP:  extractStringSlice(data["failedIp"]),
			}
			if ed.ErrorMsg != "" {
				msg += fmt.Sprintf(", errorMsg=%s", ed.ErrorMsg)
			}
			if ed.ErrorCode != nil && fmt.Sprint(ed.ErrorCode) != "0" && fmt.Sprint(ed.ErrorCode) != "<nil>" {
				msg += fmt.Sprintf(", errorCode=%v", ed.ErrorCode)
			}
			if len(ed.FailedIP) > 0 {
				msg += fmt.Sprintf(", failedIp=[%s]", strings.Join(ed.FailedIP, ", "))
			}
		} else if m := strings.TrimSpace(fmt.Sprint(resp["msg"])); m != "" && m != "<nil>" {
			msg += fmt.Sprintf(": %s", m)
		}
		return fmt.Errorf("%s", msg)
	}

	taskID := taskIDFromResponse(resp)
	if taskID == "" {
		log.Printf("响应中未找到 taskId，视为本批同步成功")
		for _, host := range batch {
			status[strings.TrimSpace(host["server_ip"])] = "success"
		}
		return nil
	}

	log.Printf("批任务已创建: taskId=%s", taskID)
	finalData, err := pollUntilComplete(ctx, client, taskID, pollInterval, pollTimeout)
	if err != nil {
		return err
	}

	failedIPs := map[string]struct{}{}
	for _, ip := range finalData.FailedIP {
		failedIPs[ip] = struct{}{}
	}

	successCount := 0
	failedCount := 0
	for _, host := range batch {
		ip := strings.TrimSpace(host["server_ip"])
		if _, ok := failedIPs[ip]; ok {
			status[ip] = "failed"
			failedCount++
		} else {
			status[ip] = "success"
			successCount++
		}
	}
	log.Printf("本批完成: 成功 %d, 失败 %d", successCount, failedCount)
	return nil
}

func pollUntilComplete(
	ctx context.Context,
	client *insightopen.Client,
	taskID string,
	pollInterval time.Duration,
	pollTimeout time.Duration,
) (*hostTaskResponse, error) {
	log.Printf("开始轮询 taskId=%s，间隔 %ds，超时 %ds", taskID, int(pollInterval.Seconds()), int(pollTimeout.Seconds()))
	deadline := time.Now().Add(pollTimeout)

	for time.Now().Before(deadline) {
		var resp insightopen.APIResponse
		path := fmt.Sprintf("/open_api/insight/external/host/querybatchAddHostResult?taskId=%s", taskID)
		if err := client.GetJSON(ctx, path, nil, &resp); err != nil {
			return nil, err
		}

		data, err := insightopen.DecodeData[hostTaskResponse](resp)
		if err != nil {
			return nil, err
		}

		if resp.Code == 1 && (data.Result == "success" || data.Result == "fail") {
			log.Printf("任务完成: result=%s, installStatus=%v", data.Result, data.InstallStatus)
			if data.Result == "fail" {
				log.Printf("errorCode=%v, errorMsg=%s", data.ErrorCode, data.ErrorMsg)
				if len(data.FailedIP) > 0 {
					log.Printf("失败 IP: %s", strings.Join(data.FailedIP, ", "))
				}
			}
			return &data, nil
		}

		log.Printf("进度: %v%% (result=%s)", data.InstallStatus, data.Result)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return nil, fmt.Errorf("任务轮询超时 (%ds), taskId=%s", int(pollTimeout.Seconds()), taskID)
}

func buildBatchPayload(hosts []map[string]string, sshPort int, sshUser, sshPasswordB64 string) []map[string]any {
	payload := make([]map[string]any, 0, len(hosts))
	for _, host := range hosts {
		dataPath := strings.TrimSpace(host["data_path"])
		if dataPath == "" {
			dataPath = "/"
		}
		installPath := strings.TrimSpace(host["install_path"])
		if installPath == "" {
			installPath = "/"
		}
		systemParameter := strings.TrimSpace(host["system_parameter"])
		if systemParameter == "" {
			systemParameter = "1"
		}
		payload = append(payload, map[string]any{
			"roomName":        strings.TrimSpace(host["room_name"]),
			"ip":              strings.TrimSpace(host["server_ip"]),
			"sshPort":         sshPort,
			"superUser":       sshUser,
			"superPwd":        sshPasswordB64,
			"installPath":     installPath,
			"dataPath":        dataPath,
			"label":           firstNonEmpty(strings.TrimSpace(host["region"])),
			"systemParameter": systemParameter,
		})
	}
	return payload
}

func chunkHosts(hosts []map[string]string, batchSize int) ([][]map[string]string, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch_size 必须大于 0")
	}
	out := make([][]map[string]string, 0, (len(hosts)+batchSize-1)/batchSize)
	for i := 0; i < len(hosts); i += batchSize {
		end := i + batchSize
		if end > len(hosts) {
			end = len(hosts)
		}
		out = append(out, hosts[i:end])
	}
	return out, nil
}

func resolveSSHPasswordB64(sshPassword, sshPasswordB64 string) string {
	if strings.TrimSpace(sshPasswordB64) != "" {
		return strings.TrimSpace(sshPasswordB64)
	}
	if sshPassword != "" {
		return base64.StdEncoding.EncodeToString([]byte(sshPassword))
	}
	log.Printf("未显式提供 SSH 密码，使用默认 base64 测试密码串")
	return defaultSSHPasswordB64
}

func taskIDFromResponse(resp map[string]any) string {
	if data, ok := resp["data"].(map[string]any); ok {
		if value := strings.TrimSpace(fmt.Sprint(data["taskId"])); value != "" && value != "<nil>" {
			return value
		}
	}
	value := strings.TrimSpace(fmt.Sprint(resp["taskId"]))
	if value == "" || value == "<nil>" {
		return ""
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func extractStringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
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

func buildPrecheckOutput(results []hostchecker.CheckResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		item := map[string]any{
			"ip":     r.IP,
			"passed": r.Passed,
		}
		if r.SysInfo != nil {
			virtType := "virtual"
			if r.SysInfo.Virt == "none" {
				virtType = "physical"
			}
			item["os"] = r.SysInfo.OS
			item["arch"] = r.SysInfo.CPUArch
			item["type"] = virtType
			item["cpu"] = r.SysInfo.CPU
			item["mem_gb"] = r.SysInfo.MemGB
		}
		if !r.Passed {
			item["reasons"] = r.Reasons
		}
		out = append(out, item)
	}
	return out
}
