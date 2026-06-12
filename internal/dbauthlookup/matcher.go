package dbauthlookup

import "sort"

func buildReport(dataset Dataset, options Options) Report {
	businessRows := filterBusinessRows(dataset.BusinessClusters, options.BusinessName)
	dbByCluster := indexDBByCluster(dataset.DBClusters)
	accessByDB := indexAccessByDB(dataset.AccessRelations)
	ipByAppCenter, ipByApp := indexAppIPs(dataset.AppIPs)

	rows := []ResultRow{}
	diagnostics := append([]Diagnostic{}, dataset.Warnings...)
	databaseSet := map[string]bool{}
	clusterSet := map[string]bool{}
	appSet := map[string]bool{}

	if len(businessRows) == 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Type:    "missing_business",
			Message: "no rows matched business name: " + options.BusinessName,
		})
	}

	for _, business := range businessRows {
		clusterSet[business.ClusterName] = true
		dbRows := dbByCluster[business.ClusterName]
		if len(dbRows) == 0 {
			fallbackDB := trimPrefixClusterDBName(business.ClusterName)
			if fallbackDB != "" {
				dbRows = []DBClusterRow{{
					ClusterName: business.ClusterName,
					DBNameRaw:   fallbackDB,
					DBName:      fallbackDB,
					DBType:      business.DBType,
				}}
				diagnostics = append(diagnostics, Diagnostic{
					Type:    "fallback_cluster_database",
					Message: "cluster did not match 数据库和集群映射表, fallback database name from cluster: " + business.ClusterName,
					Source:  business.ClusterName,
				})
			} else {
				diagnostics = append(diagnostics, Diagnostic{
					Type:    "missing_cluster_mapping",
					Message: "cluster not found in 数据库和集群映射表: " + business.ClusterName,
					Source:  business.ClusterName,
				})
				continue
			}
		}
		for _, db := range dbRows {
			databaseSet[db.DBName] = true
			accessRows := accessByDB[db.DBName]
			if len(accessRows) == 0 {
				diagnostics = append(diagnostics, Diagnostic{
					Type:    "missing_access_relation",
					Message: "database not found in 访问关系表: " + db.DBName,
					Source:  db.DBName,
				})
				continue
			}
			for _, access := range accessRows {
				appSet[access.ApplicationName] = true
				ips := ipByAppCenter[appKey(access.ApplicationName, access.ApplicationCenter)]
				if len(ips) == 0 {
					ips = ipByApp[simpleAppKey(access.ApplicationName)]
				}
				row := ResultRow{
					BusinessName:      business.BusinessName,
					DBType:            firstNonEmpty(business.DBType, db.DBType),
					ClusterName:       business.ClusterName,
					PrimaryHost:       business.PrimaryHost,
					DBName:            db.DBName,
					ApplicationName:   access.ApplicationName,
					ApplicationCenter: access.ApplicationCenter,
					DBPrimaryCenter:   access.DBPrimaryCenter,
					DBRole:            access.DBRole,
					IPs:               ips,
					DBUser:            access.DBUser,
					Privilege:         access.Privilege,
					Remark:            access.Remark,
					MatchStatus:       "matched",
				}
				if len(ips) == 0 {
					row.MatchStatus = "missing_ip_mapping"
					row.Warning = "application not found in 应用和ip映射表"
					diagnostics = append(diagnostics, Diagnostic{
						Type:    "missing_ip_mapping",
						Message: "application not found in 应用和ip映射表: " + access.ApplicationName,
						Source:  access.ApplicationName,
					})
				}
				rows = append(rows, row)
			}
		}
	}
	sortResultRows(rows)
	report := Report{
		BusinessName: options.BusinessName,
		Summary: Summary{
			BusinessClusterRows: len(businessRows),
			DatabaseCount:       len(databaseSet),
			ClusterCount:        len(clusterSet),
			ApplicationCount:    len(appSet),
			AuthorizationCount:  len(rows),
			DiagnosticCount:     len(diagnostics),
		},
		Rows: rows,
	}
	if options.WithDiagnostics {
		report.Diagnostics = diagnostics
	}
	return report
}

func filterBusinessRows(rows []BusinessClusterRow, businessName string) []BusinessClusterRow {
	target := cleanText(businessName)
	result := []BusinessClusterRow{}
	for _, row := range rows {
		if cleanText(row.BusinessName) == target {
			result = append(result, row)
		}
	}
	return result
}

func indexDBByCluster(rows []DBClusterRow) map[string][]DBClusterRow {
	index := map[string][]DBClusterRow{}
	for _, row := range rows {
		index[cleanText(row.ClusterName)] = append(index[cleanText(row.ClusterName)], row)
	}
	return index
}

func indexAccessByDB(rows []AccessRelationRow) map[string][]AccessRelationRow {
	index := map[string][]AccessRelationRow{}
	for _, row := range rows {
		index[normalizeDBName(row.DBName)] = append(index[normalizeDBName(row.DBName)], row)
	}
	return index
}

func indexAppIPs(rows []AppIPRow) (map[string][]string, map[string][]string) {
	byCenter := map[string][]string{}
	byName := map[string][]string{}
	for _, row := range rows {
		byCenter[appKey(row.ApplicationName, row.ApplicationCenter)] = appendUnique(byCenter[appKey(row.ApplicationName, row.ApplicationCenter)], row.IPs...)
		byName[simpleAppKey(row.ApplicationName)] = appendUnique(byName[simpleAppKey(row.ApplicationName)], row.IPs...)
	}
	return byCenter, byName
}

func appendUnique(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sortResultRows(rows []ResultRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ClusterName != rows[j].ClusterName {
			return rows[i].ClusterName < rows[j].ClusterName
		}
		if rows[i].DBName != rows[j].DBName {
			return rows[i].DBName < rows[j].DBName
		}
		if rows[i].ApplicationName != rows[j].ApplicationName {
			return rows[i].ApplicationName < rows[j].ApplicationName
		}
		return rows[i].DBRole < rows[j].DBRole
	})
}
