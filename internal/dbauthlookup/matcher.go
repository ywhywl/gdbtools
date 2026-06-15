package dbauthlookup

import "sort"

func buildReport(dataset Dataset, options Options) Report {
	businessRows := filterBusinessRows(dataset.BusinessClusters, options.BusinessNames)
	dbByCluster := indexDBByCluster(dataset.DBClusters)
	accessByDB := indexAccessByDB(dataset.AccessRelations)
	ipByAppCenter, ipByApp := indexAppIPs(dataset.AppIPs)

	rows := []ResultRow{}
	diagnostics := append([]Diagnostic{}, dataset.Warnings...)
	databaseSet := map[string]bool{}
	clusterSet := map[string]bool{}
	appSet := map[string]bool{}

	if len(businessRows) == 0 && len(options.BusinessNames) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Type:    "missing_business",
			Message: "no rows matched requested business names",
		})
	}

	for _, business := range businessRows {
		clusterSet[business.ClusterName] = true
		dbRows := dbByCluster[business.ClusterName]
		if len(dbRows) == 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Type:    "missing_cluster_mapping",
				Message: "cluster not found in 数据库和集群映射表: " + business.ClusterName,
				Source:  business.ClusterName,
			})
			continue
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
		BusinessNames: append([]string{}, options.BusinessNames...),
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
	report.Console = buildConsoleSummary(businessRows, rows)
	if options.WithDiagnostics {
		report.Diagnostics = diagnostics
	}
	return report
}

func buildConsoleSummary(businessRows []BusinessClusterRow, rows []ResultRow) ConsoleSummary {
	businesses := map[string]bool{}
	clusters := map[string]bool{}
	databases := map[string]bool{}
	applications := map[string]bool{}
	ips := map[string]bool{}
	businessClusters := map[string]map[string]bool{}
	for _, business := range businessRows {
		businesses[business.BusinessName] = true
		clusters[business.ClusterName] = true
		if businessClusters[business.BusinessName] == nil {
			businessClusters[business.BusinessName] = map[string]bool{}
		}
		businessClusters[business.BusinessName][business.ClusterName] = true
	}

	type groupStats struct {
		databases    map[string]bool
		applications map[string]bool
		ips          map[string]bool
		authCount    int
	}
	groups := map[string]*groupStats{}
	groupOrder := []string{}
	for _, row := range rows {
		businesses[row.BusinessName] = true
		clusters[row.ClusterName] = true
		databases[row.DBName] = true
		applications[row.ApplicationName] = true
		if businessClusters[row.BusinessName] == nil {
			businessClusters[row.BusinessName] = map[string]bool{}
		}
		businessClusters[row.BusinessName][row.ClusterName] = true
		for _, ip := range row.IPs {
			ips[ip] = true
		}
		key := row.BusinessName + "\x00" + row.ApplicationCenter
		if groups[key] == nil {
			groups[key] = &groupStats{
				databases:    map[string]bool{},
				applications: map[string]bool{},
				ips:          map[string]bool{},
			}
			groupOrder = append(groupOrder, key)
		}
		group := groups[key]
		group.databases[row.DBName] = true
		group.applications[row.ApplicationName] = true
		for _, ip := range row.IPs {
			group.ips[ip] = true
		}
		group.authCount++
	}
	sort.Slice(groupOrder, func(i, j int) bool {
		leftBusiness, leftCenter := splitGroupKey(groupOrder[i])
		rightBusiness, rightCenter := splitGroupKey(groupOrder[j])
		if leftBusiness != rightBusiness {
			return leftBusiness < rightBusiness
		}
		return leftCenter < rightCenter
	})
	byBusiness := make([]BusinessIDCSummary, 0, len(groupOrder))
	for _, key := range groupOrder {
		businessName, applicationCenter := splitGroupKey(key)
		group := groups[key]
		byBusiness = append(byBusiness, BusinessIDCSummary{
			BusinessName:       businessName,
			ApplicationCenter:  applicationCenter,
			ClusterCount:       len(businessClusters[businessName]),
			DatabaseCount:      len(group.databases),
			AuthorizationCount: group.authCount,
			ApplicationCount:   len(group.applications),
			IPCount:            len(group.ips),
		})
	}
	return ConsoleSummary{
		Total: ConsoleTotal{
			BusinessCount:      len(businesses),
			ClusterCount:       len(clusters),
			DatabaseCount:      len(databases),
			AuthorizationCount: len(rows),
			ApplicationCount:   len(applications),
			IPCount:            len(ips),
		},
		ByBusiness: byBusiness,
	}
}

func splitGroupKey(key string) (string, string) {
	for i, r := range key {
		if r == '\x00' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

func filterBusinessRows(rows []BusinessClusterRow, businessNames []string) []BusinessClusterRow {
	if len(businessNames) == 0 {
		return append([]BusinessClusterRow{}, rows...)
	}
	targets := map[string]bool{}
	for _, businessName := range businessNames {
		targets[cleanText(businessName)] = true
	}
	result := []BusinessClusterRow{}
	for _, row := range rows {
		if targets[cleanText(row.BusinessName)] {
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
