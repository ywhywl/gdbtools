package insightopen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

type PollOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

func StartInstallTask(ctx context.Context, client *Client, endpoint string, payload any) (string, error) {
	var resp APIResponse
	if err := client.PostJSON(ctx, endpoint, payload, &resp); err != nil {
		return "", err
	}
	if resp.Code != 1 {
		raw, _ := json.Marshal(resp)
		return "", fmt.Errorf(string(raw))
	}

	data, err := DecodeData[map[string]any](resp)
	if err != nil {
		return "", fmt.Errorf("decode task response failed: %w", err)
	}
	taskID := firstNonEmpty(toString(data["taskId"]))
	if taskID == "" {
		return "", fmt.Errorf("接口未返回 taskId: %s", string(resp.Data))
	}
	return taskID, nil
}

func PollTaskResult(ctx context.Context, client *Client, endpoint, taskID string, options PollOptions) (map[string]any, error) {
	interval := options.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = time.Hour
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var resp APIResponse
		query := url.Values{}
		query.Set("taskId", taskID)
		if err := client.GetJSON(ctx, endpoint, query, &resp); err != nil {
			return nil, err
		}
		if resp.Code == 1 {
			data, err := DecodeData[map[string]any](resp)
			if err != nil {
				return nil, err
			}
			if toInt(data["totalResult"]) == 1 {
				return data, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
	return nil, fmt.Errorf("任务轮询超时: taskId=%s", taskID)
}
