//go:build ignore

package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

func main() {
	dir := "sample_excels"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}

	files := map[string][][]string{
		"数据库集群映射表": {
			{"部门", "业务名称", "数据库类型", "集群名", "主库", "备库", "临时备", "同城备", "异地备"},
			{"交易清算部", "gdb-trans", "goldendb", "BJ13_clearing_branch_00", "10.26.24.11", "10.26.24.12", "", "", ""},
			{"交易清算部", "gdb-trans", "goldendb", "BJ13_clearing_branch_01", "10.26.25.11", "10.26.25.12", "", "", ""},
		},
		"数据库和集群映射表": {
			{"集群名", "数据库名称", "数据库类型"},
			{"BJ13_clearing_branch_00", "clearing_branch_00", "NUDB"},
			{"BJ13_clearing_branch_01", "clearing_branch_01", "NUDB"},
		},
		"访问关系表": {
			{"序号", "应用名称-CMDB", "应用所属中心", "数据库名称", "数据库主库所属中心", "目标节点数据库角色", "访问数据库使用用户", "访问权限", "备注"},
			{"1", "clearing-worker", "BJ13", "clearing_branch_00至clearing_branch_01", "bj13", "主库", "nclearingwork", "select,insert,update", "交易写入"},
			{"2", "clearing-report", "12", "clearing_branch_00", "BJ13", "主库", "nclearingrpt", "select", "报表查询"},
		},
		"应用和ip映射表": {
			{"应用名称-CMDB", "应用所属中心", "IP"},
			{"clearing-worker", "13", "172.0.10.2,172.0.10.3"},
			{"clearing-report", "12", "172.0.20.2"},
		},
	}

	for name, rows := range files {
		if err := writeXLSX(filepath.Join(dir, name+".xlsx"), rows); err != nil {
			panic(err)
		}
		if err := writeCSV(filepath.Join(dir, name+".csv"), rows); err != nil {
			panic(err)
		}
	}
	fmt.Printf("generated %d db-auth-lookup sample tables in %s\n", len(files), dir)
}

func writeXLSX(path string, rows [][]string) error {
	file := excelize.NewFile()
	sheet := file.GetSheetName(file.GetActiveSheetIndex())
	if sheet == "" {
		sheet = "Sheet1"
	}
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
	for colIndex := range rows[0] {
		colName, err := excelize.ColumnNumberToName(colIndex + 1)
		if err != nil {
			return err
		}
		if err := file.SetColWidth(sheet, colName, colName, 18); err != nil {
			return err
		}
	}
	if err := file.SaveAs(path); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeCSV(path string, rows [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	if err := writer.WriteAll(rows); err != nil {
		_ = file.Close()
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
