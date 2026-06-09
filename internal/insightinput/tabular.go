package insightinput

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ParseTabularInput(path string) ([]map[string]string, error) {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return ParseJSONInput(path)
	}
	return ParseCSVInput(path)
}

func ParseCSVInput(path string) ([]map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	sample := string(content)
	delimiter := detectDelimiter(sample)

	reader := csv.NewReader(strings.NewReader(sample))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	headers := make([]string, len(records[0]))
	for i, header := range records[0] {
		headers[i] = strings.TrimSpace(header)
	}

	rows := make([]map[string]string, 0, max(len(records)-1, 0))
	for _, record := range records[1:] {
		row := map[string]string{}
		nonEmpty := false
		for i, header := range headers {
			value := ""
			if i < len(record) {
				value = strings.TrimSpace(record[i])
			}
			row[header] = value
			if value != "" {
				nonEmpty = true
			}
		}
		if nonEmpty {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func ParseJSONInput(path string) ([]map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	items, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("JSON 文件必须是对象数组")
	}

	rows := make([]map[string]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("JSON 数组元素必须是对象")
		}
		row := map[string]string{}
		for key, value := range object {
			if value == nil {
				row[key] = ""
				continue
			}
			row[key] = strings.TrimSpace(fmt.Sprint(value))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func detectDelimiter(sample string) rune {
	firstLine := ""
	for _, line := range strings.Split(sample, "\n") {
		if strings.TrimSpace(line) != "" {
			firstLine = line
			break
		}
	}

	candidates := []rune{',', '\t', ';', '|'}
	best := ','
	bestCount := -1
	for _, candidate := range candidates {
		count := strings.Count(firstLine, string(candidate))
		if count > bestCount {
			best = candidate
			bestCount = count
		}
	}
	return best
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
