package dbauthlookup

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func writeXLSXReport(report Report, path string) error {
	file := excelize.NewFile()
	detailSheet := "授权明细"
	defaultSheet := file.GetSheetName(file.GetActiveSheetIndex())
	if defaultSheet == "" {
		defaultSheet = "Sheet1"
	}
	if err := file.SetSheetName(defaultSheet, detailSheet); err != nil {
		return err
	}
	if err := writeXLSXRows(file, detailSheet, detailRows(report)); err != nil {
		return err
	}
	file.SetActiveSheet(0)
	return file.SaveAs(path)
}

func writeXLSXRows(file *excelize.File, sheet string, rows [][]string) error {
	for rowIndex, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, rowIndex+1)
		if err != nil {
			return err
		}
		values := make([]interface{}, len(row))
		for colIndex, value := range row {
			values[colIndex] = value
		}
		if err := file.SetSheetRow(sheet, cell, &values); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		return nil
	}
	endCell, err := excelize.CoordinatesToCellName(len(rows[0]), len(rows))
	if err != nil {
		return err
	}
	if err := file.SetSheetDimension(sheet, "A1:"+endCell); err != nil {
		return err
	}
	for colIndex := range rows[0] {
		colName, err := excelize.ColumnNumberToName(colIndex + 1)
		if err != nil {
			return err
		}
		if err := file.SetColWidth(sheet, colName, colName, 18); err != nil {
			return err
		}
	}
	return nil
}

func detailRows(report Report) [][]string {
	rows := [][]string{
		{"manager", "业务名称", "数据库类型", "集群名", "主库", "数据库名称", "应用名称-CMDB", "应用所属中心", "数据库主库所属中心", "目标节点数据库角色", "IP", "访问数据库使用用户", "访问权限", "备注", "状态", "告警"},
	}
	aggregateOutput := report.AggregateBy == "database" || report.AggregateBy == "cluster"
	for _, row := range report.Rows {
		rows = append(rows, []string{
			formatXLSXMultiValue(row.Manager, aggregateOutput),
			row.BusinessName,
			row.DBType,
			row.ClusterName,
			row.PrimaryHost,
			formatXLSXMultiValue(row.DBName, aggregateOutput),
			formatXLSXMultiValue(row.ApplicationName, aggregateOutput),
			formatXLSXMultiValue(row.ApplicationCenter, aggregateOutput),
			formatXLSXMultiValue(row.DBPrimaryCenter, aggregateOutput),
			formatXLSXMultiValue(row.DBRole, aggregateOutput),
			formatIPs(row.IPs),
			formatXLSXMultiValue(row.DBUser, aggregateOutput),
			row.Privilege,
			formatXLSXMultiValue(row.Remark, aggregateOutput),
			formatXLSXMultiValue(row.MatchStatus, aggregateOutput),
			formatXLSXMultiValue(row.Warning, aggregateOutput),
		})
	}
	return rows
}

func formatIPs(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	result := ips[0]
	for _, ip := range ips[1:] {
		result = fmt.Sprintf("%s\n%s", result, ip)
	}
	return result
}

func formatXLSXMultiValue(value string, aggregateOutput bool) string {
	if !aggregateOutput {
		return value
	}
	return strings.ReplaceAll(value, ",", "\n")
}
