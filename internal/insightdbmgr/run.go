package insightdbmgr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ywhywl/gdbtools/internal/insightopen"
)

func Run(args []string) (int, error) {
	fs := flag.NewFlagSet("insight-create-dbmgr", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var api string
	var clusterName string
	var userName string
	var userHost string
	var password string
	var passwordB64 string
	var grantFile string
	var remarks string
	var verifySSL bool

	fs.StringVar(&api, "api", "", "Insight 地址，格式 host:port 或完整 URL")
	fs.StringVar(&clusterName, "cluster-name", "", "集群名称")
	fs.StringVar(&userName, "user-name", "dbmgr", "用户名，默认 dbmgr")
	fs.StringVar(&userHost, "user-host", "%", "客户端 host，默认 %")
	fs.StringVar(&password, "password", "", "密码明文")
	fs.StringVar(&passwordB64, "password-b64", "", "密码 base64")
	fs.StringVar(&grantFile, "grant-file", "", "授权语句 JSON 文件")
	fs.StringVar(&remarks, "remarks", "", "备注")
	fs.BoolVar(&verifySSL, "verify-ssl", false, "启用 SSL 证书校验；默认关闭")

	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if strings.TrimSpace(api) == "" {
		return 2, fmt.Errorf("--api is required")
	}
	if strings.TrimSpace(clusterName) == "" {
		return 2, fmt.Errorf("--cluster-name is required")
	}
	if strings.TrimSpace(grantFile) == "" {
		return 2, fmt.Errorf("--grant-file is required")
	}

	client, err := insightopen.NewClient(api, !verifySSL)
	if err != nil {
		return 2, err
	}

	clusterID, err := insightopen.SearchClusterID(context.Background(), client, clusterName)
	if err != nil {
		return 2, err
	}
	resolvedPasswordB64, err := resolvePasswordB64(password, passwordB64)
	if err != nil {
		return 2, err
	}
	grantList, err := loadGrants(grantFile)
	if err != nil {
		return 2, err
	}

	payload := []map[string]any{{
		"clusterId":  clusterID,
		"userName":   userName,
		"userHost":   userHost,
		"userPasswd": resolvedPasswordB64,
		"grantList":  grantList,
		"remarks":    remarks,
	}}

	var resp map[string]any
	if err := client.PostJSON(context.Background(), "/open_api/insight/external/addDBUserAndGrant", payload, &resp); err != nil {
		return 2, err
	}

	output, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return 2, err
	}
	fmt.Println(string(output))

	if toInt(resp["code"]) == 1 {
		return 0, nil
	}
	return 1, nil
}

func resolvePasswordB64(password, passwordB64 string) (string, error) {
	if strings.TrimSpace(passwordB64) != "" {
		return strings.TrimSpace(passwordB64), nil
	}
	if password == "" {
		return "", fmt.Errorf("必须提供 --password 或 --password-b64")
	}
	return base64.StdEncoding.EncodeToString([]byte(password)), nil
}

func loadGrants(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var arrayData []any
	if err := json.Unmarshal(content, &arrayData); err == nil {
		grants := normalizeGrants(arrayData)
		if len(grants) == 0 {
			return nil, fmt.Errorf("grant 文件不能为空")
		}
		return grants, nil
	}

	var objectData map[string]any
	if err := json.Unmarshal(content, &objectData); err != nil {
		return nil, err
	}

	items, ok := objectData["grantList"].([]any)
	if !ok {
		return nil, fmt.Errorf("grant 文件必须是字符串数组，或包含 grantList 数组的对象")
	}
	grants := normalizeGrants(items)
	if len(grants) == 0 {
		return nil, fmt.Errorf("grant 文件不能为空")
	}
	return grants, nil
}

func normalizeGrants(items []any) []string {
	grants := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value != "" {
			grants = append(grants, value)
		}
	}
	return grants
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
