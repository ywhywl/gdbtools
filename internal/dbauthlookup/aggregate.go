package dbauthlookup

import (
	"sort"
	"strings"
)

func aggregateRows(rows []ResultRow, aggregateBy string) []ResultRow {
	switch aggregateBy {
	case "database":
		return aggregateRowsByKey(rows, databaseAggregateKey)
	case "cluster":
		return aggregateRowsByKey(rows, clusterAggregateKey)
	default:
		return rows
	}
}

func databaseAggregateKey(row ResultRow) string {
	return strings.Join([]string{
		row.Manager,
		row.BusinessName,
		row.DBType,
		row.ClusterName,
		row.PrimaryHost,
		row.DBName,
		row.ApplicationName,
	}, "\x00")
}

func clusterAggregateKey(row ResultRow) string {
	return strings.Join([]string{
		row.Manager,
		row.BusinessName,
		row.DBType,
		row.ClusterName,
		row.PrimaryHost,
	}, "\x00")
}

func aggregateRowsByKey(rows []ResultRow, keyFunc func(ResultRow) string) []ResultRow {
	type accumulator struct {
		row                ResultRow
		dbNames            map[string]bool
		applicationNames   map[string]bool
		applicationCenters map[string]bool
		dbPrimaryCenters   map[string]bool
		dbRoles            map[string]bool
		ips                map[string]bool
		dbUsers            map[string]bool
		privileges         map[string]bool
		matchStatuses      map[string]bool
		warnings           map[string]bool
		remarks            map[string]bool
	}

	groups := map[string]*accumulator{}
	order := []string{}
	for _, row := range rows {
		key := keyFunc(row)
		group := groups[key]
		if group == nil {
			group = &accumulator{
				row:                row,
				dbNames:            map[string]bool{},
				applicationNames:   map[string]bool{},
				applicationCenters: map[string]bool{},
				dbPrimaryCenters:   map[string]bool{},
				dbRoles:            map[string]bool{},
				ips:                map[string]bool{},
				dbUsers:            map[string]bool{},
				privileges:         map[string]bool{},
				matchStatuses:      map[string]bool{},
				warnings:           map[string]bool{},
				remarks:            map[string]bool{},
			}
			groups[key] = group
			order = append(order, key)
		}
		addString(group.dbNames, row.DBName)
		addString(group.applicationNames, row.ApplicationName)
		addString(group.applicationCenters, row.ApplicationCenter)
		addString(group.dbPrimaryCenters, row.DBPrimaryCenter)
		addString(group.dbRoles, row.DBRole)
		addStrings(group.ips, row.IPs)
		addString(group.dbUsers, row.DBUser)
		addString(group.privileges, row.Privilege)
		addString(group.matchStatuses, row.MatchStatus)
		addString(group.warnings, row.Warning)
		addString(group.remarks, row.Remark)
	}

	result := make([]ResultRow, 0, len(order))
	for _, key := range order {
		group := groups[key]
		row := group.row
		row.DBName = joinSet(group.dbNames, ",")
		row.ApplicationName = joinSet(group.applicationNames, ",")
		row.ApplicationCenter = joinSet(group.applicationCenters, ",")
		row.DBPrimaryCenter = joinSet(group.dbPrimaryCenters, ",")
		row.DBRole = joinSet(group.dbRoles, ",")
		row.IPs = sortedSet(group.ips)
		row.DBUser = joinSet(group.dbUsers, ",")
		row.Privilege = joinSet(group.privileges, ",")
		row.MatchStatus = joinSet(group.matchStatuses, ",")
		row.Warning = joinSet(group.warnings, ",")
		row.Remark = joinSet(group.remarks, ",")
		result = append(result, row)
	}
	sortResultRows(result)
	return result
}

func addString(values map[string]bool, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values[value] = true
}

func addStrings(values map[string]bool, additions []string) {
	for _, value := range additions {
		addString(values, value)
	}
}

func joinSet(values map[string]bool, sep string) string {
	return strings.Join(sortedSet(values), sep)
}

func sortedSet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
