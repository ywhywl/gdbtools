package goldendb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func NewClient(options ClientOptions) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if options.Auth.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if options.Registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	options.Registry.buildIndexes()

	var httpClient HTTPDoer = newDefaultHTTPClient(nil)
	if options.HTTPClient != nil {
		httpClient = options.HTTPClient
	}

	return &Client{
		baseURL:  baseURL,
		auth:     options.Auth,
		http:     httpClient,
		registry: options.Registry,
	}, nil
}

func (c *Client) Registry() *Registry {
	return c.registry
}

func (c *Client) InvokeTool(ctx context.Context, toolName string, input InvokeInput) (*InvokeResult, error) {
	tool, ok := c.registry.FindTool(toolName)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}
	return c.invokeTool(ctx, tool, input, true)
}

func (c *Client) invokeTool(ctx context.Context, tool Tool, input InvokeInput, normalizeCluster bool) (*InvokeResult, error) {
	if normalizeCluster {
		var err error
		input, err = c.normalizeInvokeInput(ctx, input)
		if err != nil {
			return nil, err
		}
	}

	requestURL, err := buildRequestURL(c.baseURL, tool.Path, input.Query)
	if err != nil {
		return nil, err
	}
	body, contentType, err := buildRequestBody(tool.Method, input.Body)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Accept":   "application/json",
		"username": c.auth.Username,
		"password": base64.StdEncoding.EncodeToString([]byte(c.auth.Password)),
	}
	for key, value := range input.Headers {
		headers[key] = value
	}

	response, err := c.http.DoWithContext(ctx, &HTTPRequest{
		Method:      tool.Method,
		URL:         requestURL,
		Header:      headers,
		Body:        body,
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke tool %s failed: %w", tool.Name, err)
	}

	result := &InvokeResult{
		Tool:       tool,
		StatusCode: response.StatusCode,
		RawBody:    response.Body,
	}
	var envelope APIResponse
	if len(response.Body) > 0 {
		if err := json.Unmarshal(response.Body, &envelope); err == nil {
			result.Response = &envelope
		}
	}
	return result, nil
}

func (c *Client) normalizeInvokeInput(ctx context.Context, input InvokeInput) (InvokeInput, error) {
	normalized := input

	query, err := normalizeClusterPayload(ctx, c, cloneMap(input.Query))
	if err != nil {
		return InvokeInput{}, err
	}
	if typed, ok := query.(map[string]any); ok || query == nil {
		normalized.Query = typed
	}

	body, err := normalizeBodyPayload(ctx, c, input.Body)
	if err != nil {
		return InvokeInput{}, err
	}
	normalized.Body = body

	return normalized, nil
}

func (c *Client) QueryTask(ctx context.Context, actionToolName, taskID string) (*InvokeResult, error) {
	pair, ok := c.registry.FindAsyncPair(actionToolName)
	if !ok {
		return nil, fmt.Errorf("async pair not found for tool: %s", actionToolName)
	}

	queryTool, ok := c.registry.FindTool(pair.Query)
	if !ok {
		return nil, fmt.Errorf("query tool not found: %s", pair.Query)
	}

	input := InvokeInput{}
	if queryTool.Method == "GET" {
		input.Query = map[string]any{pair.Key: taskID}
	} else {
		input.Body = map[string]any{pair.Key: taskID}
	}
	return c.InvokeTool(ctx, pair.Query, input)
}

func (c *Client) PollTask(ctx context.Context, actionToolName, taskID string, options PollOptions) (*InvokeResult, error) {
	interval := options.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	maxTries := options.MaxTries
	if maxTries <= 0 {
		maxTries = 30
	}

	var last *InvokeResult
	for attempt := 1; attempt <= maxTries; attempt++ {
		result, err := c.QueryTask(ctx, actionToolName, taskID)
		if err != nil {
			return nil, err
		}
		last = result

		if options.Stop != nil {
			stop, err := options.Stop(result)
			if err != nil {
				return nil, err
			}
			if stop {
				return result, nil
			}
		} else if result.Response != nil && result.Response.Code != 0 {
			return result, nil
		}

		if attempt == maxTries {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}

	if last == nil {
		return nil, fmt.Errorf("poll task finished without any result")
	}
	return last, nil
}

func buildRequestURL(baseURL, path string, query map[string]any) (string, error) {
	fullURL := baseURL + path
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return "", fmt.Errorf("invalid request URL %s: %w", fullURL, err)
	}
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, fmt.Sprint(value))
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func buildRequestBody(method string, body any) ([]byte, string, error) {
	if method == "GET" || body == nil {
		return nil, "", nil
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request body failed: %w", err)
	}
	return payload, "application/json;charset=UTF-8", nil
}

func normalizeBodyPayload(ctx context.Context, client *Client, body any) (any, error) {
	if body == nil {
		return nil, nil
	}

	switch typed := body.(type) {
	case map[string]any:
		return normalizeClusterPayload(ctx, client, cloneMap(typed))
	default:
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body for normalization failed: %w", err)
		}
		var generic any
		if err := json.Unmarshal(payload, &generic); err != nil {
			return nil, fmt.Errorf("unmarshal request body for normalization failed: %w", err)
		}
		return normalizeClusterPayload(ctx, client, generic)
	}
}

func normalizeClusterPayload(ctx context.Context, client *Client, payload any) (any, error) {
	switch typed := payload.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		if hasNonEmptyValue(typed, "clusterId") {
			return normalizeNestedClusterFields(ctx, client, typed, false)
		}
		clusterName, ok := getNonEmptyString(typed, "clusterName")
		if ok {
			clusterID, err := client.lookupClusterIDByName(ctx, clusterName)
			if err != nil {
				return nil, err
			}
			delete(typed, "clusterName")
			typed["clusterId"] = clusterID
		}
		return normalizeNestedClusterFields(ctx, client, typed, false)
	case []any:
		return normalizeNestedClusterFields(ctx, client, typed, true)
	default:
		return payload, nil
	}
}

func normalizeNestedClusterFields(ctx context.Context, client *Client, payload any, descend bool) (any, error) {
	if !descend {
		typed := payload.(map[string]any)
		for key, value := range typed {
			normalized, err := normalizeClusterPayload(ctx, client, value)
			if err != nil {
				return nil, err
			}
			typed[key] = normalized
		}
		return typed, nil
	}

	items := payload.([]any)
	for idx, value := range items {
		normalized, err := normalizeClusterPayload(ctx, client, value)
		if err != nil {
			return nil, err
		}
		items[idx] = normalized
	}
	return items, nil
}

func (c *Client) lookupClusterIDByName(ctx context.Context, clusterName string) (any, error) {
	tool, ok := c.registry.FindTool("tenancy_query_clusters")
	if !ok {
		return nil, fmt.Errorf("tool not found: tenancy_query_clusters")
	}

	result, err := c.invokeTool(ctx, tool, InvokeInput{}, false)
	if err != nil {
		return nil, fmt.Errorf("query cluster list for %q failed: %w", clusterName, err)
	}
	if result.Response == nil {
		return nil, fmt.Errorf("query cluster list for %q returned empty response", clusterName)
	}
	if result.Response.Code != 1 {
		return nil, fmt.Errorf("query cluster list for %q failed: %s", clusterName, strings.TrimSpace(result.Response.Msg))
	}

	clusters, err := parseClusterList(result.Response.Data)
	if err != nil {
		return nil, fmt.Errorf("parse cluster list for %q failed: %w", clusterName, err)
	}

	var matches []clusterRecord
	for _, cluster := range clusters {
		if cluster.TenancyName == clusterName {
			matches = append(matches, cluster)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("cluster name %q not found", clusterName)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("cluster name %q matched %d clusters, please use clusterId", clusterName, len(matches))
	}
	if matches[0].ClusterID == nil {
		return nil, fmt.Errorf("cluster name %q found but clusterId is empty", clusterName)
	}
	return matches[0].ClusterID, nil
}

type clusterRecord struct {
	TenancyName string
	ClusterID   any
}

func parseClusterList(raw json.RawMessage) ([]clusterRecord, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	items := extractClusterItems(payload)
	records := make([]clusterRecord, 0, len(items))
	for _, item := range items {
		name, _ := getNonEmptyString(item, "tenancyName")
		clusterID, ok := item["clusterId"]
		if !ok {
			continue
		}
		records = append(records, clusterRecord{
			TenancyName: name,
			ClusterID:   clusterID,
		})
	}
	return records, nil
}

func extractClusterItems(payload any) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, value := range typed {
			if item, ok := value.(map[string]any); ok {
				items = append(items, item)
			}
		}
		return items
	case map[string]any:
		for _, key := range []string{"list", "rows", "items", "data"} {
			if nested, ok := typed[key]; ok {
				if items := extractClusterItems(nested); len(items) > 0 {
					return items
				}
			}
		}
		return nil
	default:
		return nil
	}
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func hasNonEmptyValue(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func getNonEmptyString(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}
