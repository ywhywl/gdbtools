package insightopen

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type searchTenancyData struct {
	List []map[string]any `json:"list"`
}

func SearchClusterID(ctx context.Context, client *Client, clusterName string) (int, error) {
	if strings.TrimSpace(clusterName) == "" {
		return 0, fmt.Errorf("cluster_name 不能为空")
	}

	var resp APIResponse
	query := url.Values{}
	query.Set("tenancyName", clusterName)
	if err := client.GetJSON(ctx, "/open_api/insight/container/tenancy/searchTenancy", query, &resp); err != nil {
		return 0, err
	}
	if resp.Code != 1 {
		return 0, fmt.Errorf(firstNonEmpty(resp.Msg, "查询集群失败: "+clusterName))
	}

	data, err := DecodeData[searchTenancyData](resp)
	if err != nil {
		return 0, fmt.Errorf("集群查询返回格式异常: %w", err)
	}

	var exactMatches []map[string]any
	for _, item := range data.List {
		tenancyName := strings.TrimSpace(toString(item["tenancyName"]))
		clusterNameValue := strings.TrimSpace(toString(item["clusterName"]))
		if clusterName == tenancyName || clusterName == clusterNameValue {
			exactMatches = append(exactMatches, item)
		}
	}

	if len(exactMatches) == 1 {
		return clusterIDFromMap(exactMatches[0], clusterName)
	}
	if len(exactMatches) == 0 && len(data.List) == 1 {
		return clusterIDFromMap(data.List[0], clusterName)
	}
	if len(exactMatches) > 1 || len(data.List) > 1 {
		return 0, fmt.Errorf("集群名 %s 匹配到多条记录，请改用更精确名称", clusterName)
	}
	return 0, fmt.Errorf("未找到集群: %s", clusterName)
}

func clusterIDFromMap(item map[string]any, clusterName string) (int, error) {
	value, ok := item["clusterId"]
	if !ok || value == nil || toString(value) == "" {
		value = item["id"]
	}
	if value == nil || toString(value) == "" {
		return 0, fmt.Errorf("集群 %s 未返回 clusterId", clusterName)
	}

	var clusterID int
	switch typed := value.(type) {
	case float64:
		clusterID = int(typed)
	case int:
		clusterID = typed
	default:
		_, err := fmt.Sscanf(toString(value), "%d", &clusterID)
		if err != nil {
			return 0, fmt.Errorf("集群 %s 返回无效 clusterId: %v", clusterName, value)
		}
	}
	return clusterID, nil
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func toInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		var out int
		fmt.Sscanf(fmt.Sprint(value), "%d", &out)
		return out
	}
}
