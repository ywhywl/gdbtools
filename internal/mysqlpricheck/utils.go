package mysqlpricheck

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var defaultExcludedSchemas = []string{"information_schema", "mysql", "performance_schema", "sys"}

func splitMultiValue(values []string) []string {
	items := []string{}
	if len(values) == 0 {
		return items
	}
	re := regexp.MustCompile(`[,，;\|；\n]+`)
	for _, raw := range values {
		for _, item := range re.Split(raw, -1) {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
	}
	return items
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func uniqSorted(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	uniq := map[string]struct{}{}
	for _, item := range items {
		uniq[item] = struct{}{}
	}
	out := make([]string, 0, len(uniq))
	for item := range uniq {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func matchesSelector(name, selector string) bool {
	matched, err := path.Match(selector, name)
	if err != nil {
		return false
	}
	return matched
}

func filterNames(availableNames, selectors, excludeSelectors []string) []string {
	selected := uniqSorted(availableNames)
	if len(selectors) > 0 {
		selected = resolveSelectors(selected, selectors)
	}
	if len(excludeSelectors) == 0 {
		return selected
	}
	excluded := map[string]struct{}{}
	for _, name := range resolveSelectors(selected, excludeSelectors) {
		excluded[name] = struct{}{}
	}
	filtered := make([]string, 0, len(selected))
	for _, name := range selected {
		if _, found := excluded[name]; !found {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func resolveSelectors(availableNames, selectors []string) []string {
	selected := []string{}
	for _, selector := range selectors {
		if contains(availableNames, selector) {
			selected = append(selected, selector)
			continue
		}
		for _, name := range availableNames {
			if matchesSelector(name, selector) {
				selected = append(selected, name)
			}
		}
	}
	return uniqSorted(selected)
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func privilegeNameFromColumn(columnName string) string {
	stem := columnName
	if strings.HasSuffix(stem, "_priv") {
		stem = stem[:len(stem)-5]
	}
	return strings.ToUpper(strings.ReplaceAll(stem, "_", " "))
}

func parsePrivilegeSet(value string) []string {
	if value == "" {
		return nil
	}
	uniq := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		uniq[strings.ToUpper(strings.ReplaceAll(trimmed, "_", " "))] = struct{}{}
	}
	privileges := make([]string, 0, len(uniq))
	for privilege := range uniq {
		privileges = append(privileges, privilege)
	}
	sort.Strings(privileges)
	return privileges
}

func parseConnectionTarget(rawTarget string, creds DefaultsFileConfig, defaultPort int) (ConnectionConfig, error) {
	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return ConnectionConfig{}, newUsageError("empty connection target")
	}
	if strings.Contains(rawTarget, "://") {
		parsed, err := url.Parse(rawTarget)
		if err != nil {
			return ConnectionConfig{}, err
		}
		if parsed.Scheme != "mysql" {
			return ConnectionConfig{}, newUsageError("unsupported DSN scheme: " + parsed.Scheme)
		}
		user := creds.User
		if parsed.User != nil && parsed.User.Username() != "" {
			username, err := url.QueryUnescape(parsed.User.Username())
			if err != nil {
				return ConnectionConfig{}, err
			}
			user = username
		}
		if user == "" {
			return ConnectionConfig{}, newUsageError("DSN must include a username or provide one via --user/--defaults-file/--config")
		}
		password := creds.Password
		if parsed.User != nil {
			if providedPassword, ok := parsed.User.Password(); ok {
				password, err = url.QueryUnescape(providedPassword)
				if err != nil {
					return ConnectionConfig{}, err
				}
			}
		}
		host := parsed.Hostname()
		if host == "" {
			host = creds.Host
		}
		if host == "" {
			host = "127.0.0.1"
		}
		port := defaultPort
		if port == 0 {
			port = 3306
		}
		if parsed.Port() != "" {
			parsedPort, err := strconv.Atoi(parsed.Port())
			if err != nil {
				return ConnectionConfig{}, err
			}
			port = parsedPort
		}
		socket := creds.Socket
		if (parsed.Hostname() == "" || parsed.Hostname() == "localhost") && parsed.RawQuery != "" {
			for _, item := range strings.Split(parsed.RawQuery, "&") {
				key, value, found := strings.Cut(item, "=")
				if !found || key != "socket" {
					continue
				}
				decoded, err := url.QueryUnescape(value)
				if err != nil {
					return ConnectionConfig{}, err
				}
				socket = decoded
				host = "localhost"
				break
			}
		}
		return ConnectionConfig{
			DSN:      rawTarget,
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Socket:   socket,
		}, nil
	}

	host := rawTarget
	port := defaultPort
	if port == 0 {
		port = 3306
	}
	if strings.Contains(rawTarget, ":") {
		parsedHost, parsedPort, err := net.SplitHostPort(rawTarget)
		if err == nil {
			host = parsedHost
			port, err = strconv.Atoi(parsedPort)
			if err != nil {
				return ConnectionConfig{}, err
			}
		} else {
			lastColon := strings.LastIndex(rawTarget, ":")
			if lastColon <= 0 || lastColon == len(rawTarget)-1 {
				return ConnectionConfig{}, newUsageError("invalid address: " + rawTarget)
			}
			host = rawTarget[:lastColon]
			port, err = strconv.Atoi(rawTarget[lastColon+1:])
			if err != nil {
				return ConnectionConfig{}, err
			}
		}
	}
	if host == "" {
		host = creds.Host
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if creds.User == "" {
		return ConnectionConfig{}, newUsageError("target must include a username or provide one via --user/--defaults-file/--config")
	}
	return ConnectionConfig{
		DSN:      rawTarget,
		Host:     host,
		Port:     port,
		User:     creds.User,
		Password: creds.Password,
		Socket:   creds.Socket,
	}, nil
}

func parseTargetList(rawValues []string, creds DefaultsFileConfig, defaultPort int) ([]ConnectionConfig, error) {
	targets := splitMultiValue(rawValues)
	if len(targets) == 0 {
		return nil, newUsageError("at least one target is required")
	}
	configs := make([]ConnectionConfig, 0, len(targets))
	for _, rawTarget := range targets {
		config, err := parseConnectionTarget(rawTarget, creds, defaultPort)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func loadFileConfig(path string) (FileConfig, error) {
	if strings.TrimSpace(path) == "" {
		return FileConfig{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}
	var config FileConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return FileConfig{}, fmt.Errorf("parse config file failed: %w", err)
	}
	return config, nil
}

func mustAtoi(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
