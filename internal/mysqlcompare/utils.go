package mysqlcompare

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var systemSchemas = map[string]struct{}{
	"information_schema": {},
	"mysql":              {},
	"performance_schema": {},
	"sys":                {},
}

func splitMultiValue(values []string) []string {
	items := []string{}
	if len(values) == 0 {
		return items
	}
	re := regexp.MustCompile(`[,\|\n]+`)
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

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func mysqlLikeToRegexp(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("^")
	escape := false
	for _, char := range pattern {
		if escape {
			builder.WriteString(regexp.QuoteMeta(string(char)))
			escape = false
			continue
		}
		switch char {
		case '\\':
			escape = true
		case '%':
			builder.WriteString(".*")
		case '_':
			builder.WriteString(".")
		default:
			builder.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	if escape {
		builder.WriteString(regexp.QuoteMeta(`\`))
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func matchesSelector(name, selector string) bool {
	re, err := mysqlLikeToRegexp(selector)
	if err != nil {
		return false
	}
	return re.MatchString(name)
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

func sqlLiteral(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case string:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `'`, `\'`) + "'"
	default:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(v), `\`, `\\`), `'`, `\'`) + "'"
	}
}

func parseNullableString(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}

func parseNullableNormalizedString(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := normalizeWhitespace(value)
	return &copyValue
}

func parseNullableInt(value string) *int {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseNullableBool(value string) *bool {
	if value == "" {
		return nil
	}
	result := value == "YES" || value == "Y" || value == "1"
	return &result
}

func normalizeExtra(extra string) string {
	if extra == "" {
		return ""
	}
	re := regexp.MustCompile(`(?i)\bauto_increment\b`)
	return normalizeWhitespace(re.ReplaceAllString(extra, ""))
}

func privilegeNameFromColumn(columnName string) string {
	stem := columnName
	if strings.HasSuffix(stem, "_priv") {
		stem = stem[:len(stem)-5]
	}
	return strings.ToUpper(strings.ReplaceAll(stem, "_", " "))
}

func parseConnectionDSN(rawDSN, label, defaultUser, defaultPassword string) (ConnectionConfig, error) {
	rawDSN = strings.TrimSpace(rawDSN)
	if rawDSN == "" {
		return ConnectionConfig{}, newUsageError("empty connection target")
	}
	if !strings.Contains(rawDSN, "://") {
		return parseSimpleAddress(rawDSN, label, defaultUser, defaultPassword)
	}
	parsed, err := url.Parse(rawDSN)
	if err != nil {
		return ConnectionConfig{}, err
	}
	if parsed.Scheme != "mysql" {
		return ConnectionConfig{}, newUsageError("unsupported DSN scheme: " + parsed.Scheme)
	}
	user := defaultUser
	if parsed.User != nil && parsed.User.Username() != "" {
		username, err := url.QueryUnescape(parsed.User.Username())
		if err != nil {
			return ConnectionConfig{}, err
		}
		user = username
	}
	if user == "" {
		return ConnectionConfig{}, newUsageError("DSN must include a username")
	}
	password := defaultPassword
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
		host = "127.0.0.1"
	}
	port := 3306
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil {
			return ConnectionConfig{}, err
		}
	}
	socket := ""
	if (parsed.Hostname() == "" || parsed.Hostname() == "localhost") && parsed.RawQuery != "" {
		for _, item := range strings.Split(parsed.RawQuery, "&") {
			key, value, found := strings.Cut(item, "=")
			if !found {
				continue
			}
			if key == "socket" {
				socket, err = url.QueryUnescape(value)
				if err != nil {
					return ConnectionConfig{}, err
				}
				host = "localhost"
				break
			}
		}
	}

	return ConnectionConfig{
		DSN:      rawDSN,
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Database: strings.TrimPrefix(parsed.Path, "/"),
		Socket:   socket,
		Label:    label,
	}, nil
}

func parseSimpleAddress(rawAddress, label, defaultUser, defaultPassword string) (ConnectionConfig, error) {
	host := rawAddress
	port := 3306

	if strings.Contains(rawAddress, ":") {
		parsedHost, parsedPort, err := net.SplitHostPort(rawAddress)
		if err == nil {
			host = parsedHost
			port, err = strconv.Atoi(parsedPort)
			if err != nil {
				return ConnectionConfig{}, err
			}
		} else {
			lastColon := strings.LastIndex(rawAddress, ":")
			if lastColon <= 0 || lastColon == len(rawAddress)-1 {
				return ConnectionConfig{}, newUsageError("invalid address: " + rawAddress)
			}
			host = rawAddress[:lastColon]
			port, err = strconv.Atoi(rawAddress[lastColon+1:])
			if err != nil {
				return ConnectionConfig{}, err
			}
		}
	}

	if defaultUser == "" {
		return ConnectionConfig{}, newUsageError("DSN must include a username or provide one via --default-user/--config")
	}
	return ConnectionConfig{
		DSN:      rawAddress,
		Host:     host,
		Port:     port,
		User:     defaultUser,
		Password: defaultPassword,
		Label:    label,
	}, nil
}

func parseTargetDSNs(rawValues []string, defaultUser, defaultPassword string) ([]ConnectionConfig, error) {
	dsns := splitMultiValue(rawValues)
	if len(dsns) == 0 {
		return nil, newUsageError("at least one target DSN is required")
	}
	targets := make([]ConnectionConfig, 0, len(dsns))
	for _, rawDSN := range dsns {
		config, err := parseConnectionDSN(rawDSN, "", defaultUser, defaultPassword)
		if err != nil {
			return nil, err
		}
		targets = append(targets, config)
	}
	return targets, nil
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

func toString(value any) string {
	return fmt.Sprint(value)
}

func mustAtoi(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
