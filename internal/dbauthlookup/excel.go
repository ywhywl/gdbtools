package dbauthlookup

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func loadDataset(options Options) (Dataset, error) {
	var dataset Dataset
	var err error
	dataset.BusinessClusters, err = loadBusinessClusterRows(options.BusinessClusterFile)
	if err != nil {
		return Dataset{}, err
	}
	dataset.DBClusters, err = loadDBClusterRows(options.DBClusterFile)
	if err != nil {
		return Dataset{}, err
	}
	dataset.AccessRelations, dataset.Warnings, err = loadAccessRelationRows(options.AccessRelationFile)
	if err != nil {
		return Dataset{}, err
	}
	dataset.AppIPs, err = loadAppIPRows(options.AppIPFile)
	if err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}

func loadBusinessClusterRows(path string) ([]BusinessClusterRow, error) {
	rows, err := readTabularRecords(path)
	if err != nil {
		return nil, err
	}
	result := make([]BusinessClusterRow, 0, len(rows))
	for _, row := range rows {
		item := BusinessClusterRow{
			Department:    valueOf(row, "部门"),
			BusinessName:  valueOf(row, "业务名称"),
			Manager:       valueOf(row, "manager"),
			DBType:        valueOf(row, "数据库类型"),
			ClusterName:   valueOf(row, "集群名"),
			PrimaryHost:   valueOf(row, "主库"),
			StandbyHost:   valueOf(row, "备库"),
			TempStandby:   valueOf(row, "临时备"),
			LocalStandby:  valueOf(row, "同城备"),
			RemoteStandby: valueOf(row, "异地备"),
		}
		if item.BusinessName == "" && item.ClusterName == "" {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func loadDBClusterRows(path string) ([]DBClusterRow, error) {
	rows, err := readTabularRecords(path)
	if err != nil {
		return nil, err
	}
	result := make([]DBClusterRow, 0, len(rows))
	for _, row := range rows {
		raw := valueOf(row, "数据库名称")
		names, err := expandDBNames(raw)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			item := DBClusterRow{
				ClusterName: valueOf(row, "集群名"),
				DBNameRaw:   raw,
				DBName:      name,
				DBType:      valueOf(row, "数据库类型"),
			}
			if item.ClusterName == "" && item.DBName == "" {
				continue
			}
			result = append(result, item)
		}
	}
	return result, nil
}

func loadAccessRelationRows(path string) ([]AccessRelationRow, []Diagnostic, error) {
	rows, err := readTabularRecords(path)
	if err != nil {
		return nil, nil, err
	}
	result := make([]AccessRelationRow, 0, len(rows))
	diagnostics := []Diagnostic{}
	for _, row := range rows {
		raw := valueOf(row, "数据库名称")
		names, err := expandDBNames(raw)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Type:    "invalid_database_range",
				Message: err.Error(),
				Source:  raw,
			})
			continue
		}
		for _, name := range names {
			item := AccessRelationRow{
				Seq:               valueOf(row, "序号"),
				ApplicationName:   valueOf(row, "应用名称-CMDB"),
				ApplicationCenter: normalizeIDC(valueOf(row, "应用所属中心")),
				DBNameRaw:         raw,
				DBName:            name,
				DBPrimaryCenter:   normalizeIDC(valueOf(row, "数据库主库所属中心")),
				DBRole:            valueOf(row, "目标节点数据库角色"),
				DBUser:            valueOf(row, "访问数据库使用用户"),
				Privilege:         valueOf(row, "访问权限"),
				Remark:            valueOf(row, "备注"),
			}
			if item.ApplicationName == "" && item.DBName == "" {
				continue
			}
			result = append(result, item)
		}
	}
	return result, diagnostics, nil
}

func loadAppIPRows(path string) ([]AppIPRow, error) {
	rows, err := readTabularRecords(path)
	if err != nil {
		return nil, err
	}
	result := make([]AppIPRow, 0, len(rows))
	for _, row := range rows {
		item := AppIPRow{
			ApplicationName:   valueOf(row, "应用名称-CMDB"),
			ApplicationCenter: normalizeIDC(valueOf(row, "应用所属中心")),
			IPs:               splitIPs(valueOf(row, "IP")),
		}
		if item.ApplicationName == "" && len(item.IPs) == 0 {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func readTabularRecords(path string) ([]map[string]string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".xlsm":
		return readExcelRecords(path)
	case ".csv":
		return readCSVRecords(path)
	default:
		return nil, fmt.Errorf("unsupported table file extension %q for %s, expected .xlsx, .xlsm, or .csv", filepath.Ext(path), path)
	}
}

func readExcelRecords(path string) ([]map[string]string, error) {
	file, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("%s has no sheets", path)
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read %s sheet %s: %w", path, sheets[0], err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return recordsFromRows(rows), nil
}

func readCSVRecords(path string) ([]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return recordsFromRows(rows), nil
}

func recordsFromRows(rows [][]string) []map[string]string {
	headers := make([]string, len(rows[0]))
	for i, header := range rows[0] {
		headers[i] = cleanText(header)
	}
	records := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		record := map[string]string{}
		empty := true
		for i, header := range headers {
			if header == "" {
				continue
			}
			value := ""
			if i < len(row) {
				value = strings.TrimSpace(row[i])
			}
			if value != "" {
				empty = false
			}
			record[header] = value
		}
		if !empty {
			records = append(records, record)
		}
	}
	return records
}

func valueOf(row map[string]string, key string) string {
	return strings.TrimSpace(row[cleanText(key)])
}

func splitIPs(value string) []string {
	value = strings.ReplaceAll(value, "，", ",")
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == ' '
	})
	ips := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		ip := strings.TrimSpace(field)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
	}
	return ips
}
