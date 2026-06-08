package goldendb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRegistryFindsTool(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join("..", "..", "docs", "goldendb_tool_interface_set.yaml"))
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}
	tool, ok := registry.FindTool("rest_tree_list")
	if !ok {
		t.Fatalf("expected rest_tree_list to exist")
	}
	if tool.Method != "GET" || tool.Path != "/open_api/insight/container/rest/tree" {
		t.Fatalf("unexpected tool: %#v", tool)
	}
}

func TestClientInvokeToolAddsHeadersAndBody(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "db_user_add_with_grant", Method: "POST", Path: "/open_api/insight/external/addDBUserAndGrant"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/open_api/insight/external/addDBUserAndGrant" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}
		if req.Header["username"] != "admin" {
			t.Fatalf("unexpected username header: %s", req.Header["username"])
		}
		wantPassword := base64.StdEncoding.EncodeToString([]byte("secret"))
		if req.Header["password"] != wantPassword {
			t.Fatalf("unexpected password header: %s", req.Header["password"])
		}
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		if body["user"] != "app" {
			t.Fatalf("unexpected request body: %#v", body)
		}
		return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success", Data: mustRawJSON(t, map[string]any{"taskId": "123"})}), nil
	}})

	result, err := client.InvokeTool(context.Background(), "db_user_add_with_grant", InvokeInput{
		Body: map[string]any{"user": "app"},
	})
	if err != nil {
		t.Fatalf("InvokeTool returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientQueryTaskUsesAsyncMapping(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "action", Method: "POST", Path: "/action"},
		{Name: "query", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/query" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}
		if parsed.Query().Get("taskId") != "task-1" {
			t.Fatalf("unexpected taskId: %s", parsed.Query().Get("taskId"))
		}
		return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
	}})
	client.registry.AsyncPairs = []AsyncPair{{Action: "action", Query: "query", Key: "taskId"}}
	client.registry.buildIndexes()

	result, err := client.QueryTask(context.Background(), "action", "task-1")
	if err != nil {
		t.Fatalf("QueryTask returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestClientQueryTaskUsesBodyForNonGETQueryTool(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "action", Method: "POST", Path: "/action"},
		{Name: "query", Method: "POST", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/query" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		if got := body["taskId"]; got != "task-2" {
			t.Fatalf("unexpected taskId: %#v", got)
		}
		return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
	}})
	client.registry.AsyncPairs = []AsyncPair{{Action: "action", Query: "query", Key: "taskId"}}
	client.registry.buildIndexes()

	result, err := client.QueryTask(context.Background(), "action", "task-2")
	if err != nil {
		t.Fatalf("QueryTask returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestClientPollTaskStopsOnPredicate(t *testing.T) {
	attempts := 0
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "query", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		attempts++
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/query" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}

		code := 2
		msg := "running"
		if attempts >= 2 {
			code = 1
			msg = "done"
		}
		return jsonHTTPResponse(t, APIResponse{Code: code, Msg: msg}), nil
	}})
	client.registry.AsyncPairs = []AsyncPair{{Action: "action", Query: "query", Key: "taskId"}}
	client.registry.buildIndexes()

	result, err := client.PollTask(context.Background(), "action", "task-1", PollOptions{
		Interval: 10 * time.Millisecond,
		MaxTries: 3,
		Stop: func(result *InvokeResult) (bool, error) {
			return result.Response != nil && result.Response.Msg == "done", nil
		},
	})
	if err != nil {
		t.Fatalf("PollTask returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if result.Response == nil || !strings.EqualFold(result.Response.Msg, "done") {
		t.Fatalf("unexpected poll result: %#v", result)
	}
}

func TestClientPollTaskStopsOnDefaultSuccessCode(t *testing.T) {
	attempts := 0
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "query", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		attempts++
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/query" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}

		code := 0
		msg := "pending"
		if attempts >= 3 {
			code = 1
			msg = "success"
		}
		return jsonHTTPResponse(t, APIResponse{Code: code, Msg: msg}), nil
	}})
	client.registry.AsyncPairs = []AsyncPair{{Action: "action", Query: "query", Key: "taskId"}}
	client.registry.buildIndexes()

	result, err := client.PollTask(context.Background(), "action", "task-3", PollOptions{
		Interval: 10 * time.Millisecond,
		MaxTries: 5,
	})
	if err != nil {
		t.Fatalf("PollTask returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected poll result: %#v", result)
	}
}

func TestClientInvokeToolResolvesClusterNameInQuery(t *testing.T) {
	clusterQueryCalls := 0
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "query_tool", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		switch parsed.Path {
		case "/open_api/insight/external/tenant/getTenancyList":
			clusterQueryCalls++
			return jsonHTTPResponse(t, APIResponse{
				Code: 1,
				Msg:  "success",
				Data: mustRawJSON(t, []map[string]any{
					{"tenancyName": "cluster-a", "clusterId": 1001},
				}),
			}), nil
		case "/query":
			if got := parsed.Query().Get("clusterId"); got != "1001" {
				t.Fatalf("unexpected clusterId: %s", got)
			}
			if got := parsed.Query().Get("clusterName"); got != "" {
				t.Fatalf("clusterName should be removed, got %s", got)
			}
			return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
		default:
			t.Fatalf("unexpected path: %s", parsed.Path)
			return nil, nil
		}
	}})

	result, err := client.InvokeTool(context.Background(), "query_tool", InvokeInput{
		Query: map[string]any{
			"clusterName": "cluster-a",
			"dbName":      "appdb",
		},
	})
	if err != nil {
		t.Fatalf("InvokeTool returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if clusterQueryCalls != 1 {
		t.Fatalf("expected 1 cluster lookup, got %d", clusterQueryCalls)
	}
}

func TestClientInvokeToolResolvesClusterNameInBody(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "action_tool", Method: "POST", Path: "/action"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		switch parsed.Path {
		case "/open_api/insight/external/tenant/getTenancyList":
			return jsonHTTPResponse(t, APIResponse{
				Code: 1,
				Msg:  "success",
				Data: mustRawJSON(t, []map[string]any{
					{"tenancyName": "cluster-b", "clusterId": 2002},
				}),
			}), nil
		case "/action":
			var body map[string]any
			if err := json.Unmarshal(req.Body, &body); err != nil {
				t.Fatalf("decode request body failed: %v", err)
			}
			if _, ok := body["clusterName"]; ok {
				t.Fatalf("clusterName should be removed from body: %#v", body)
			}
			if got := body["clusterId"]; got != float64(2002) {
				t.Fatalf("unexpected clusterId: %#v", got)
			}
			return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
		default:
			t.Fatalf("unexpected path: %s", parsed.Path)
			return nil, nil
		}
	}})

	result, err := client.InvokeTool(context.Background(), "action_tool", InvokeInput{
		Body: map[string]any{
			"clusterName": "cluster-b",
			"dbName":      "appdb",
		},
	})
	if err != nil {
		t.Fatalf("InvokeTool returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientInvokeToolResolvesNestedClusterNameInBody(t *testing.T) {
	clusterQueryCalls := 0
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "action_tool", Method: "POST", Path: "/action"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		switch parsed.Path {
		case "/open_api/insight/external/tenant/getTenancyList":
			clusterQueryCalls++
			return jsonHTTPResponse(t, APIResponse{
				Code: 1,
				Msg:  "success",
				Data: mustRawJSON(t, []map[string]any{
					{"tenancyName": "cluster-c", "clusterId": 3003},
				}),
			}), nil
		case "/action":
			var body map[string]any
			if err := json.Unmarshal(req.Body, &body); err != nil {
				t.Fatalf("decode request body failed: %v", err)
			}
			spec, ok := body["spec"].(map[string]any)
			if !ok {
				t.Fatalf("unexpected body: %#v", body)
			}
			targets, ok := spec["targets"].([]any)
			if !ok || len(targets) != 1 {
				t.Fatalf("unexpected targets: %#v", spec["targets"])
			}
			target, ok := targets[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected target: %#v", targets[0])
			}
			if _, ok := target["clusterName"]; ok {
				t.Fatalf("clusterName should be removed from nested body: %#v", target)
			}
			if got := target["clusterId"]; got != float64(3003) {
				t.Fatalf("unexpected clusterId: %#v", got)
			}
			return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
		default:
			t.Fatalf("unexpected path: %s", parsed.Path)
			return nil, nil
		}
	}})

	result, err := client.InvokeTool(context.Background(), "action_tool", InvokeInput{
		Body: map[string]any{
			"spec": map[string]any{
				"targets": []any{
					map[string]any{
						"clusterName": "cluster-c",
						"dbName":      "appdb",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("InvokeTool returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if clusterQueryCalls != 1 {
		t.Fatalf("expected 1 cluster lookup, got %d", clusterQueryCalls)
	}
}

func TestClientInvokeToolKeepsExplicitClusterID(t *testing.T) {
	clusterQueryCalls := 0
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "query_tool", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		switch parsed.Path {
		case "/open_api/insight/external/tenant/getTenancyList":
			clusterQueryCalls++
			t.Fatalf("cluster lookup should not be called when clusterId is provided")
			return nil, nil
		case "/query":
			if got := parsed.Query().Get("clusterId"); got != "9001" {
				t.Fatalf("unexpected clusterId: %s", got)
			}
			return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
		default:
			t.Fatalf("unexpected path: %s", parsed.Path)
			return nil, nil
		}
	}})

	_, err := client.InvokeTool(context.Background(), "query_tool", InvokeInput{
		Query: map[string]any{
			"clusterId": "9001",
		},
	})
	if err != nil {
		t.Fatalf("InvokeTool returned error: %v", err)
	}
	if clusterQueryCalls != 0 {
		t.Fatalf("expected 0 cluster lookups, got %d", clusterQueryCalls)
	}
}

func TestClientInvokeToolErrorsOnUnknownClusterName(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "query_tool", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/open_api/insight/external/tenant/getTenancyList" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}
		return jsonHTTPResponse(t, APIResponse{
			Code: 1,
			Msg:  "success",
			Data: mustRawJSON(t, []map[string]any{
				{"tenancyName": "cluster-x", "clusterId": 1},
			}),
		}), nil
	}})

	_, err := client.InvokeTool(context.Background(), "query_tool", InvokeInput{
		Query: map[string]any{
			"clusterName": "cluster-missing",
		},
	})
	if err == nil || !strings.Contains(err.Error(), `cluster name "cluster-missing" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientInvokeToolErrorsOnDuplicateClusterName(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "query_tool", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		if parsed.Path != "/open_api/insight/external/tenant/getTenancyList" {
			t.Fatalf("unexpected path: %s", parsed.Path)
		}
		return jsonHTTPResponse(t, APIResponse{
			Code: 1,
			Msg:  "success",
			Data: mustRawJSON(t, []map[string]any{
				{"tenancyName": "cluster-dup", "clusterId": 1},
				{"tenancyName": "cluster-dup", "clusterId": 2},
			}),
		}), nil
	}})

	_, err := client.InvokeTool(context.Background(), "query_tool", InvokeInput{
		Query: map[string]any{
			"clusterName": "cluster-dup",
		},
	})
	if err == nil || !strings.Contains(err.Error(), `cluster name "cluster-dup" matched 2 clusters`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientInvokeToolMatchesClusterNameExactly(t *testing.T) {
	client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
		{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
		{Name: "query_tool", Method: "GET", Path: "/query"},
	}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
		parsed := mustParseURL(t, req.URL)
		switch parsed.Path {
		case "/open_api/insight/external/tenant/getTenancyList":
			return jsonHTTPResponse(t, APIResponse{
				Code: 1,
				Msg:  "success",
				Data: mustRawJSON(t, []map[string]any{
					{"tenancyName": "cluster-a", "clusterId": 11},
					{"tenancyName": "cluster-a-prod", "clusterId": 22},
				}),
			}), nil
		case "/query":
			if got := parsed.Query().Get("clusterId"); got != "11" {
				t.Fatalf("unexpected clusterId: %s", got)
			}
			return jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"}), nil
		default:
			t.Fatalf("unexpected path: %s", parsed.Path)
			return nil, nil
		}
	}})

	result, err := client.InvokeTool(context.Background(), "query_tool", InvokeInput{
		Query: map[string]any{
			"clusterName": "cluster-a",
		},
	})
	if err != nil {
		t.Fatalf("InvokeTool returned error: %v", err)
	}
	if result.Response == nil || result.Response.Code != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestParseClusterListAcceptsTopLevelArray(t *testing.T) {
	records, err := parseClusterList(mustRawJSON(t, []map[string]any{
		{"tenancyName": "cluster-a", "clusterId": 1},
	}))
	if err != nil {
		t.Fatalf("parseClusterList returned error: %v", err)
	}
	if len(records) != 1 || records[0].TenancyName != "cluster-a" || records[0].ClusterID != float64(1) {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestParseClusterListAcceptsWrappedList(t *testing.T) {
	records, err := parseClusterList(mustRawJSON(t, map[string]any{
		"list": []map[string]any{
			{"tenancyName": "cluster-a", "clusterId": 1},
		},
	}))
	if err != nil {
		t.Fatalf("parseClusterList returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("unexpected records length: %#v", records)
	}
}

func TestParseClusterListHandlesNull(t *testing.T) {
	records, err := parseClusterList(json.RawMessage("null"))
	if err != nil {
		t.Fatalf("parseClusterList returned error: %v", err)
	}
	if records != nil {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestClientCommandLineVerificationScenarios(t *testing.T) {
	t.Run("invoke_tool_with_body", func(t *testing.T) {
		client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
			{Name: "db_user_add_with_grant", Method: "POST", Path: "/open_api/insight/external/addDBUserAndGrant"},
		}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
			t.Logf("HTTP request: method=%s url=%s headers=%s body=%s", req.Method, req.URL, mustJSON(t, req.Header), string(req.Body))
			response := jsonHTTPResponse(t, APIResponse{
				Code: 1,
				Msg:  "success",
				Data: mustRawJSON(t, map[string]any{"taskId": "123"}),
			})
			t.Logf("HTTP response: status=%d body=%s", response.StatusCode, string(response.Body))
			return response, nil
		}})

		input := InvokeInput{
			Body: map[string]any{
				"user":     "app",
				"password": "secret-123",
			},
		}
		t.Logf("Invoke input: tool=%s payload=%s", "db_user_add_with_grant", mustJSON(t, input.Body))

		result, err := client.InvokeTool(context.Background(), "db_user_add_with_grant", input)
		if err != nil {
			t.Fatalf("InvokeTool returned error: %v", err)
		}
		t.Logf("Invoke result: status=%d response=%s", result.StatusCode, mustJSON(t, result.Response))
	})

	t.Run("query_task_get", func(t *testing.T) {
		client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
			{Name: "action", Method: "POST", Path: "/action"},
			{Name: "query", Method: "GET", Path: "/query"},
		}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
			t.Logf("HTTP request: method=%s url=%s headers=%s body=%s", req.Method, req.URL, mustJSON(t, req.Header), string(req.Body))
			response := jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"})
			t.Logf("HTTP response: status=%d body=%s", response.StatusCode, string(response.Body))
			return response, nil
		}})
		client.registry.AsyncPairs = []AsyncPair{{Action: "action", Query: "query", Key: "taskId"}}
		client.registry.buildIndexes()

		actionToolName := "action"
		taskID := "task-1"
		t.Logf("QueryTask input: actionTool=%s taskId=%s", actionToolName, taskID)

		result, err := client.QueryTask(context.Background(), actionToolName, taskID)
		if err != nil {
			t.Fatalf("QueryTask returned error: %v", err)
		}
		t.Logf("QueryTask result: status=%d response=%s", result.StatusCode, mustJSON(t, result.Response))
	})

	t.Run("poll_task_default_stop", func(t *testing.T) {
		attempts := 0
		client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
			{Name: "query", Method: "GET", Path: "/query"},
		}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
			attempts++
			response := jsonHTTPResponse(t, APIResponse{
				Code: func() int {
					if attempts >= 3 {
						return 1
					}
					return 0
				}(),
				Msg: func() string {
					if attempts >= 3 {
						return "success"
					}
					return "pending"
				}(),
			})
			t.Logf("HTTP request #%d: method=%s url=%s headers=%s body=%s", attempts, req.Method, req.URL, mustJSON(t, req.Header), string(req.Body))
			t.Logf("HTTP response #%d: status=%d body=%s", attempts, response.StatusCode, string(response.Body))
			return response, nil
		}})
		client.registry.AsyncPairs = []AsyncPair{{Action: "action", Query: "query", Key: "taskId"}}
		client.registry.buildIndexes()

		options := PollOptions{
			Interval: 10 * time.Millisecond,
			MaxTries: 5,
		}
		t.Logf("PollTask input: actionTool=%s taskId=%s options=%s", "action", "task-3", mustJSON(t, map[string]any{
			"interval_ms": options.Interval.Milliseconds(),
			"max_tries":   options.MaxTries,
			"stop":        "<default: response.code != 0>",
		}))

		result, err := client.PollTask(context.Background(), "action", "task-3", options)
		if err != nil {
			t.Fatalf("PollTask returned error: %v", err)
		}
		t.Logf("PollTask result: attempts=%d status=%d response=%s", attempts, result.StatusCode, mustJSON(t, result.Response))
	})

	t.Run("invoke_tool_with_cluster_name", func(t *testing.T) {
		client := newGoldenDBTestClient(t, "https://goldendb.local", []Tool{
			{Name: "tenancy_query_clusters", Method: "GET", Path: "/open_api/insight/external/tenant/getTenancyList"},
			{Name: "query_tool", Method: "GET", Path: "/query"},
		}, mockHTTPClient{do: func(_ context.Context, req *HTTPRequest) (*HTTPResponse, error) {
			t.Logf("HTTP request: method=%s url=%s headers=%s body=%s", req.Method, req.URL, mustJSON(t, req.Header), string(req.Body))
			parsed := mustParseURL(t, req.URL)
			var response *HTTPResponse
			switch parsed.Path {
			case "/open_api/insight/external/tenant/getTenancyList":
				response = jsonHTTPResponse(t, APIResponse{
					Code: 1,
					Msg:  "success",
					Data: mustRawJSON(t, []map[string]any{
						{"tenancyName": "cluster-a", "clusterId": 1001},
					}),
				})
			case "/query":
				response = jsonHTTPResponse(t, APIResponse{Code: 1, Msg: "success"})
			default:
				return nil, fmt.Errorf("unexpected path: %s", parsed.Path)
			}
			t.Logf("HTTP response: status=%d body=%s", response.StatusCode, string(response.Body))
			return response, nil
		}})

		input := InvokeInput{
			Query: map[string]any{
				"clusterName": "cluster-a",
				"dbName":      "appdb",
			},
		}
		t.Logf("Invoke input: tool=%s payload=%s", "query_tool", mustJSON(t, input.Query))

		result, err := client.InvokeTool(context.Background(), "query_tool", input)
		if err != nil {
			t.Fatalf("InvokeTool returned error: %v", err)
		}
		t.Logf("Invoke result: status=%d response=%s", result.StatusCode, mustJSON(t, result.Response))
	})
}

type mockHTTPClient struct {
	do func(ctx context.Context, req *HTTPRequest) (*HTTPResponse, error)
}

func (m mockHTTPClient) DoWithContext(ctx context.Context, req *HTTPRequest) (*HTTPResponse, error) {
	return m.do(ctx, req)
}

func newGoldenDBTestClient(t *testing.T, baseURL string, tools []Tool, httpClient HTTPDoer) *Client {
	t.Helper()

	registry := &Registry{
		ToolGroups: []ToolGroup{{
			Group: "test",
			Tools: tools,
		}},
	}
	registry.buildIndexes()

	client, err := NewClient(ClientOptions{
		BaseURL:    baseURL,
		Auth:       Auth{Username: "admin", Password: "secret"},
		Registry:   registry,
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw JSON failed: %v", err)
	}
	return payload
}

func jsonHTTPResponse(t *testing.T, body APIResponse) *HTTPResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}
	return &HTTPResponse{
		StatusCode: 200,
		Header:     map[string][]string{"Content-Type": {"application/json"}},
		Body:       payload,
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL failed: %v", err)
	}
	return parsed
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON failed: %v", err)
	}
	return string(payload)
}
