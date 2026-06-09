package insightopen

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func ResolveDBGroupID(ctx context.Context, client *Client, clusterID int, dbgroupName string) (int, error) {
	var resp APIResponse
	query := url.Values{}
	query.Set("clusterId", fmt.Sprintf("%d", clusterID))
	if err := client.GetJSON(ctx, "/open_api/insight/external/tenant/getGroup", query, &resp); err != nil {
		return 0, err
	}
	if resp.Code != 1 {
		return 0, fmt.Errorf(firstNonEmpty(resp.Msg, fmt.Sprintf("查询分片失败: %d", clusterID)))
	}

	data, err := DecodeData[[]map[string]any](resp)
	if err != nil {
		return 0, fmt.Errorf("分片查询返回格式异常: %w", err)
	}

	matches := make([]map[string]any, 0)
	for _, item := range data {
		if strings.TrimSpace(toString(item["groupName"])) == strings.TrimSpace(dbgroupName) {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		return 0, fmt.Errorf("分片名 %s 匹配结果异常: %d", dbgroupName, len(matches))
	}
	return toInt(matches[0]["groupId"]), nil
}
