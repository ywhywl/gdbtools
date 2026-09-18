package insightopen

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func NormalizeAPIBase(api string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(api), "/")
	if raw == "" {
		return "", fmt.Errorf("api 不能为空")
	}

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("无效的 api 地址: %s", api)
		}
		return parsed.Scheme + "://" + parsed.Host, nil
	}

	parsed, err := url.Parse("//" + raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("无效的 api 地址: %s", api)
	}

	scheme := "https"
	switch parsed.Hostname() {
	case "127.0.0.1", "localhost", "0.0.0.0":
		scheme = "http"
	}
	return scheme + "://" + parsed.Host, nil
}

// NormalizeParameterTemplateName returns the template file name expected by
// the Insight install APIs. The API examples use JSON file names, while some
// older input files contain the same name without the .json suffix.
func NormalizeParameterTemplateName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" && !strings.HasSuffix(strings.ToLower(name), ".json") {
		return name + ".json"
	}
	return name
}

func DecodeData[T any](resp APIResponse) (T, error) {
	var out T
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return out, err
	}
	return out, nil
}
