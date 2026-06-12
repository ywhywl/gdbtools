package dbauthlookup

import (
	"encoding/json"
	"fmt"
	"strings"
)

func renderReport(report Report, outputFormat string) (string, error) {
	switch outputFormat {
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "text":
		return renderTextReport(report), nil
	default:
		return "", newUsageError("invalid output format")
	}
}

func renderTextReport(report Report) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Business: %s\n", report.BusinessName))
	builder.WriteString(fmt.Sprintf("Business cluster rows: %d\n", report.Summary.BusinessClusterRows))
	builder.WriteString(fmt.Sprintf("Databases: %d, Clusters: %d, Applications: %d, Authorizations: %d\n",
		report.Summary.DatabaseCount,
		report.Summary.ClusterCount,
		report.Summary.ApplicationCount,
		report.Summary.AuthorizationCount,
	))
	if report.Summary.DiagnosticCount > 0 {
		builder.WriteString(fmt.Sprintf("Diagnostics: %d (use --with-diagnostics to render details)\n", report.Summary.DiagnosticCount))
	}
	builder.WriteString("\n")
	if len(report.Rows) == 0 {
		builder.WriteString("No authorization rows matched.\n")
		return builder.String()
	}
	headers := []string{"业务名称", "数据库类型", "集群名", "主库", "数据库名称", "应用名称-CMDB", "应用所属中心", "IP", "访问数据库使用用户", "访问权限", "状态"}
	table := [][]string{headers}
	for _, row := range report.Rows {
		table = append(table, []string{
			row.BusinessName,
			row.DBType,
			row.ClusterName,
			row.PrimaryHost,
			row.DBName,
			row.ApplicationName,
			row.ApplicationCenter,
			strings.Join(row.IPs, ","),
			row.DBUser,
			row.Privilege,
			row.MatchStatus,
		})
	}
	builder.WriteString(renderTable(table))
	if len(report.Diagnostics) > 0 {
		builder.WriteString("\n\nDiagnostics:\n")
		for _, diagnostic := range report.Diagnostics {
			builder.WriteString(fmt.Sprintf("- %s: %s\n", diagnostic.Type, diagnostic.Message))
		}
	}
	return builder.String()
}

func renderTable(rows [][]string) string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if len([]rune(cell)) > widths[i] {
				widths[i] = len([]rune(cell))
			}
		}
	}
	var builder strings.Builder
	for rowIndex, row := range rows {
		for i, cell := range row {
			if i > 0 {
				builder.WriteString("  ")
			}
			builder.WriteString(padRight(cell, widths[i]))
		}
		builder.WriteString("\n")
		if rowIndex == 0 {
			for i, width := range widths {
				if i > 0 {
					builder.WriteString("  ")
				}
				builder.WriteString(strings.Repeat("-", width))
			}
			builder.WriteString("\n")
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}

func padRight(value string, width int) string {
	padding := width - len([]rune(value))
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}
