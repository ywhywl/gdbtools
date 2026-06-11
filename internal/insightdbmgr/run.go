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
	var dbUserPassword string
	var dbUserPasswordB64 string
	var dbUserPasswordFile string
	var grantFile string
	var remarks string
	var verifySSL bool
	var authFlags insightopen.AuthFlags

	fs.StringVar(&api, "api", "", "Insight 地址，格式 host:port 或完整 URL")
	fs.StringVar(&clusterName, "cluster-name", "", "集群名称")
	fs.StringVar(&userName, "user-name", "dbmgr", "用户名，默认 dbmgr")
	fs.StringVar(&userHost, "user-host", "%", "客户端 host，默认 %")
	fs.StringVar(&dbUserPassword, "db-user-password", "", "新建数据库用户密码明文")
	fs.StringVar(&dbUserPasswordB64, "db-user-password-b64", "", "新建数据库用户密码 base64")
	fs.StringVar(&dbUserPasswordFile, "db-user-password-file", "", "新建数据库用户密码映射文件(JSON)")
	fs.StringVar(&dbUserPassword, "password", "", "新建数据库用户密码明文（兼容旧参数）")
	fs.StringVar(&dbUserPasswordB64, "password-b64", "", "新建数据库用户密码 base64（兼容旧参数）")
	fs.StringVar(&grantFile, "grant-file", "", "授权语句 JSON 文件")
	fs.StringVar(&remarks, "remarks", "", "备注")
	fs.BoolVar(&verifySSL, "verify-ssl", false, "启用 SSL 证书校验；默认关闭")
	insightopen.AddAuthFlags(fs, &authFlags)

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

	auth, err := insightopen.ResolveAuth(authFlags)
	if err != nil {
		return 2, err
	}

	client, err := insightopen.NewClient(api, !verifySSL, auth)
	if err != nil {
		return 2, err
	}

	clusterID, err := insightopen.SearchClusterID(context.Background(), client, clusterName)
	if err != nil {
		return 2, err
	}
	grantList, err := loadGrants(grantFile)
	if err != nil {
		return 2, err
	}

	payload, err := buildPayload(
		clusterID,
		userName,
		userHost,
		dbUserPassword,
		dbUserPasswordB64,
		dbUserPasswordFile,
		grantList,
		remarks,
	)
	if err != nil {
		return 2, err
	}

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

type passwordFileEntry struct {
	Password    string `json:"password"`
	PasswordB64 string `json:"passwordB64"`
	Remarks     string `json:"remarks"`
}

func buildPayload(
	clusterID int,
	userName string,
	userHost string,
	dbUserPassword string,
	dbUserPasswordB64 string,
	dbUserPasswordFile string,
	grantList []string,
	remarks string,
) ([]map[string]any, error) {
	if strings.TrimSpace(dbUserPasswordFile) != "" {
		return loadPasswordFilePayload(clusterID, dbUserPasswordFile, grantList, remarks)
	}

	resolvedPasswordB64, err := resolvePasswordB64(dbUserPassword, dbUserPasswordB64)
	if err != nil {
		return nil, err
	}

	return []map[string]any{{
		"clusterId":  clusterID,
		"userName":   userName,
		"userHost":   userHost,
		"userPasswd": resolvedPasswordB64,
		"grantList":  grantList,
		"remarks":    remarks,
	}}, nil
}

func loadPasswordFilePayload(clusterID int, path string, grantList []string, defaultRemarks string) ([]map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("db-user-password-file 必须是 JSON 对象")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("db-user-password-file 不能为空")
	}

	payload := make([]map[string]any, 0, len(raw))
	for key, value := range raw {
		userName, userHost, err := splitUserHostKey(key)
		if err != nil {
			return nil, err
		}

		passwordB64, entryRemarks, err := resolvePasswordFileEntry(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		remarks := defaultRemarks
		if strings.TrimSpace(entryRemarks) != "" {
			remarks = entryRemarks
		}

		payload = append(payload, map[string]any{
			"clusterId":  clusterID,
			"userName":   userName,
			"userHost":   userHost,
			"userPasswd": passwordB64,
			"grantList":  grantList,
			"remarks":    remarks,
		})
	}
	return payload, nil
}

func splitUserHostKey(key string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(key), "@", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("db-user-password-file 键必须是 user@host 格式: %s", key)
	}
	userName := strings.TrimSpace(parts[0])
	userHost := strings.TrimSpace(parts[1])
	if userName == "" || userHost == "" {
		return "", "", fmt.Errorf("db-user-password-file 键必须是 user@host 格式: %s", key)
	}
	return userName, userHost, nil
}

func resolvePasswordFileEntry(value any) (string, string, error) {
	switch typed := value.(type) {
	case string:
		passwordB64, err := resolvePasswordB64(typed, "")
		return passwordB64, "", err
	case map[string]any:
		entry := passwordFileEntry{}
		if raw, ok := typed["password"]; ok {
			entry.Password = strings.TrimSpace(fmt.Sprint(raw))
		}
		if raw, ok := typed["passwordB64"]; ok {
			entry.PasswordB64 = strings.TrimSpace(fmt.Sprint(raw))
		}
		if raw, ok := typed["remarks"]; ok {
			entry.Remarks = strings.TrimSpace(fmt.Sprint(raw))
		}
		passwordB64, err := resolvePasswordB64(entry.Password, entry.PasswordB64)
		return passwordB64, entry.Remarks, err
	default:
		return "", "", fmt.Errorf("值必须是密码字符串，或包含 password/passwordB64 的对象")
	}
}

func resolvePasswordB64(password, passwordB64 string) (string, error) {
	if strings.TrimSpace(passwordB64) != "" {
		return strings.TrimSpace(passwordB64), nil
	}
	if password == "" {
		return "", fmt.Errorf("必须提供 --db-user-password 或 --db-user-password-b64")
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
