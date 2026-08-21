package dbauthlookup

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	fullRangePattern  = regexp.MustCompile(`^(.+?)(\d+)\s*(?:至|-|~|－|—)\s*(.+?)(\d+)$`)
	shortRangePattern = regexp.MustCompile(`^(.+?)(\d+)\s*(?:至|-|~|－|—)\s*(\d+)$`)
	idcPattern        = regexp.MustCompile(`^(?:[A-Za-z]+)?(\d{2})$`)
)

func cleanText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.ReplaceAll(value, "，", ",")
	value = strings.Join(strings.Fields(value), "")
	return value
}

func normalizeDBName(value string) string {
	return cleanText(value)
}

func normalizeIDC(value string) string {
	value = cleanText(value)
	if value == "" {
		return ""
	}
	if matches := idcPattern.FindStringSubmatch(value); matches != nil {
		return matches[1]
	}
	return value
}

func expandDBNames(raw string) ([]string, error) {
	value := normalizeDBName(raw)
	if value == "" {
		return nil, nil
	}
	if matches := shortRangePattern.FindStringSubmatch(value); matches != nil {
		return expandNumberRange(matches[1], matches[2], matches[3], raw)
	}
	if matches := fullRangePattern.FindStringSubmatch(value); matches != nil {
		startPrefix, startNum := matches[1], matches[2]
		endPrefix, endNum := matches[3], matches[4]
		if startPrefix != endPrefix {
			return nil, fmt.Errorf("database range prefix mismatch: %s", raw)
		}
		return expandNumberRange(startPrefix, startNum, endNum, raw)
	}
	return []string{value}, nil
}

func expandNumberRange(prefix, startNum, endNum, raw string) ([]string, error) {
	start, err := strconv.Atoi(startNum)
	if err != nil {
		return nil, fmt.Errorf("invalid database range start %q in %s", startNum, raw)
	}
	end, err := strconv.Atoi(endNum)
	if err != nil {
		return nil, fmt.Errorf("invalid database range end %q in %s", endNum, raw)
	}
	if start > end {
		return nil, fmt.Errorf("database range start greater than end: %s", raw)
	}
	width := len(startNum)
	names := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		names = append(names, fmt.Sprintf("%s%0*d", prefix, width, i))
	}
	return names, nil
}

func trimPrefixClusterDBName(clusterName string) string {
	value := normalizeDBName(clusterName)
	if strings.HasPrefix(value, "BJ13_") {
		return strings.TrimPrefix(value, "BJ13_")
	}
	return value
}

func appKey(name, center string) string {
	return cleanText(name) + "\x00" + normalizeIDC(center)
}

func simpleAppKey(name string) string {
	return cleanText(name)
}
