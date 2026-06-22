package dbauthlookup

import (
	"bytes"
	"encoding/csv"
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
	case "csv":
		return renderCSVReport(report)
	default:
		return "", newUsageError("invalid output format")
	}
}

func renderCSVReport(report Report) (string, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	records := [][]string{
		{"manager", "业务名称", "数据库类型", "集群名", "主库", "数据库名称", "应用名称-CMDB", "应用所属中心", "数据库主库所属中心", "目标节点数据库角色", "IP", "访问数据库使用用户", "访问权限", "备注", "状态", "告警"},
	}
	for _, row := range report.Rows {
		records = append(records, []string{
			row.Manager,
			row.BusinessName,
			row.DBType,
			row.ClusterName,
			row.PrimaryHost,
			row.DBName,
			row.ApplicationName,
			row.ApplicationCenter,
			row.DBPrimaryCenter,
			row.DBRole,
			strings.Join(row.IPs, ","),
			row.DBUser,
			row.Privilege,
			row.Remark,
			row.MatchStatus,
			row.Warning,
		})
	}
	if err := writer.WriteAll(records); err != nil {
		return "", err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return strings.TrimRight(buffer.String(), "\n"), nil
}

func renderDiagnostics(report Report) string {
	if len(report.Diagnostics) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Diagnostics:\n")
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Source == "" {
			builder.WriteString(fmt.Sprintf("- %s: %s\n", diagnostic.Type, diagnostic.Message))
			continue
		}
		builder.WriteString(fmt.Sprintf("- %s [%s]: %s\n", diagnostic.Type, diagnostic.Source, diagnostic.Message))
	}
	return builder.String()
}

func renderConsoleSummary(report Report) string {
	var builder strings.Builder
	builder.WriteString("Summary:\n")
	builder.WriteString(fmt.Sprintf("Businesses: %d\n", report.Console.Total.BusinessCount))
	builder.WriteString(fmt.Sprintf("Clusters: %d\n", report.Console.Total.ClusterCount))
	builder.WriteString(fmt.Sprintf("Databases: %d\n", report.Console.Total.DatabaseCount))
	builder.WriteString(fmt.Sprintf("Authorization rows: %d\n", report.Console.Total.AuthorizationCount))
	builder.WriteString(fmt.Sprintf("Applications: %d\n", report.Console.Total.ApplicationCount))
	builder.WriteString(fmt.Sprintf("Application IPs: %d\n", report.Console.Total.IPCount))
	builder.WriteString("\nBy business:\n")
	if len(report.Console.ByBusiness) == 0 {
		builder.WriteString("- none\n")
		return builder.String()
	}
	for _, item := range report.Console.ByBusiness {
		builder.WriteString(fmt.Sprintf("- %s: clusters=%d databases=%d idc-%s applications=%d ips=%d authorization_rows=%d\n",
			item.BusinessName,
			item.ClusterCount,
			item.DatabaseCount,
			item.ApplicationCenter,
			item.ApplicationCount,
			item.IPCount,
			item.AuthorizationCount,
		))
	}
	return builder.String()
}

func renderTextReport(report Report) string {
	var builder strings.Builder
	if len(report.BusinessNames) == 0 {
		builder.WriteString("Businesses: ALL\n")
	} else {
		builder.WriteString(fmt.Sprintf("Businesses: %s\n", strings.Join(report.BusinessNames, ", ")))
	}
	builder.WriteString(fmt.Sprintf("Aggregate by: %s\n", firstNonEmpty(report.AggregateBy, "detail")))
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
	headers := []string{"manager", "业务名称", "数据库类型", "集群名", "主库", "数据库名称", "应用名称-CMDB", "应用所属中心", "IP", "访问数据库使用用户", "访问权限", "备注", "状态"}
	table := [][]string{headers}
	for _, row := range report.Rows {
		table = append(table, []string{
			row.Manager,
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
			row.Remark,
			row.MatchStatus,
		})
	}
	builder.WriteString(renderTable(table))
	if len(report.Diagnostics) > 0 {
		builder.WriteString("\n\n")
		builder.WriteString(strings.TrimRight(renderDiagnostics(report), "\n"))
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
