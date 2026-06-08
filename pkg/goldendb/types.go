package goldendb

import (
	"context"
	"encoding/json"
	"time"
)

type Auth struct {
	Username string
	Password string
}

type ClientOptions struct {
	BaseURL    string
	Auth       Auth
	HTTPClient HTTPDoer
	Registry   *Registry
}

type HTTPDoer interface {
	DoWithContext(ctx context.Context, req *HTTPRequest) (*HTTPResponse, error)
}

type HTTPRequest struct {
	Method      string
	URL         string
	Header      map[string]string
	Body        []byte
	ContentType string
}

type HTTPResponse struct {
	StatusCode int
	Header     map[string][]string
	Body       []byte
}

type Registry struct {
	Version     int         `yaml:"version"`
	Service     ServiceInfo `yaml:"service"`
	Abstraction Abstraction `yaml:"abstraction"`
	ToolGroups  []ToolGroup `yaml:"tool_groups"`
	AsyncPairs  []AsyncPair `yaml:"async_pairs"`
	Notes       []string    `yaml:"notes"`
	toolIndex   map[string]Tool
	asyncIndex  map[string]AsyncPair
}

type ServiceInfo struct {
	Name           string         `yaml:"name"`
	SourceDocument string         `yaml:"source_document"`
	Auth           AuthSpec       `yaml:"auth"`
	CommonResponse map[string]any `yaml:"common_response"`
}

type AuthSpec struct {
	Type    string                 `yaml:"type"`
	Headers map[string]HeaderField `yaml:"headers"`
}

type HeaderField struct {
	Required    bool   `yaml:"required"`
	Type        string `yaml:"type"`
	Encoding    string `yaml:"encoding,omitempty"`
	Description string `yaml:"description"`
}

type Abstraction struct {
	Purpose           []string      `yaml:"purpose"`
	RecommendedLayers []string      `yaml:"recommended_layers"`
	GenericTools      []GenericTool `yaml:"generic_tools"`
}

type GenericTool struct {
	Name        string `yaml:"name"`
	BasedOn     string `yaml:"based_on,omitempty"`
	Description string `yaml:"description"`
}

type ToolGroup struct {
	Group   string `yaml:"group"`
	Section string `yaml:"section"`
	Tools   []Tool `yaml:"tools"`
}

type Tool struct {
	Name        string   `yaml:"name"`
	Section     string   `yaml:"section"`
	Method      string   `yaml:"method"`
	Path        string   `yaml:"path"`
	Summary     string   `yaml:"summary"`
	ReturnsTask bool     `yaml:"returns_task,omitempty"`
	Query       []string `yaml:"query,omitempty"`
	Group       string   `yaml:"-"`
}

type AsyncPair struct {
	Action string `yaml:"action"`
	Query  string `yaml:"query"`
	Key    string `yaml:"key"`
}

type InvokeInput struct {
	Query   map[string]any
	Body    any
	Headers map[string]string
}

type APIResponse struct {
	Code        int             `json:"code"`
	Msg         string          `json:"msg"`
	Data        json.RawMessage `json:"data"`
	Duration    string          `json:"duration"`
	OperationID string          `json:"operationId"`
}

type InvokeResult struct {
	Tool       Tool
	StatusCode int
	RawBody    []byte
	Response   *APIResponse
}

type PollOptions struct {
	Interval time.Duration
	MaxTries int
	Stop     func(*InvokeResult) (bool, error)
}

type Client struct {
	baseURL  string
	auth     Auth
	http     HTTPDoer
	registry *Registry
}

type Poller interface {
	PollTask(ctx context.Context, actionToolName, taskID string, options PollOptions) (*InvokeResult, error)
}
