package mysqlpricheck

import (
	"fmt"
	"sort"
	"strings"
)

func resolveUsers(client DatabaseClient, selectors, excludeSelectors []string, includeAnonymous bool) ([]UserHost, error) {
	rows, err := client.FetchRows(`
		SELECT User AS user, Host AS host
		FROM mysql.user
		ORDER BY User, Host
	`)
	if err != nil {
		return nil, err
	}
	allUsers := []UserHost{}
	for _, row := range rows {
		user := row["user"]
		host := row["host"]
		if user == "" && !includeAnonymous {
			continue
		}
		item := UserHost{User: user, Host: host}
		allUsers = append(allUsers, item)
	}

	selected := map[string]UserHost{}
	selectors = splitMultiValue(selectors)
	excludeSelectors = splitMultiValue(excludeSelectors)
	if len(selectors) == 0 {
		for _, item := range allUsers {
			selected[item.DisplayName()] = item
		}
	} else {
		for _, selector := range selectors {
			selector = strings.TrimSpace(selector)
			if selector == "" {
				continue
			}
			if strings.Contains(selector, "@") {
				for _, item := range allUsers {
					if item.DisplayName() == selector || matchesSelector(item.DisplayName(), selector) {
						selected[item.DisplayName()] = item
					}
				}
				continue
			}
			for _, item := range allUsers {
				if item.User == selector || matchesSelector(item.User, selector) {
					selected[item.DisplayName()] = item
				}
			}
		}
	}
	for _, selector := range excludeSelectors {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		for key, item := range selected {
			if strings.Contains(selector, "@") {
				if item.DisplayName() == selector || matchesSelector(item.DisplayName(), selector) {
					delete(selected, key)
				}
				continue
			}
			if item.User == selector || matchesSelector(item.User, selector) {
				delete(selected, key)
			}
		}
	}
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]UserHost, 0, len(keys))
	for _, key := range keys {
		result = append(result, selected[key])
	}
	return result, nil
}

func collectPrivilegeSnapshots(client DatabaseClient, users []UserHost, excludeSchemas []string) ([]PrivilegeSnapshot, error) {
	userColumns, err := privilegeColumns(client, "mysql.user")
	if err != nil {
		return nil, err
	}
	dbColumns, err := privilegeColumns(client, "mysql.db")
	if err != nil {
		return nil, err
	}
	identities := map[string]struct{}{}
	for _, item := range users {
		identities[item.DisplayName()] = struct{}{}
	}
	excludedSchemas := map[string]struct{}{}
	for _, name := range uniqSorted(splitMultiValue(excludeSchemas)) {
		excludedSchemas[name] = struct{}{}
	}

	snapshots := map[string]*PrivilegeSnapshot{}
	for _, item := range users {
		snapshots[item.DisplayName()] = newPrivilegeSnapshot(item)
	}

	globalRows, err := client.FetchRows(buildPrivilegeQuery("mysql.user", []string{"User AS user", "Host AS host"}, userColumns))
	if err != nil {
		return nil, err
	}
	for _, row := range globalRows {
		identity := row["user"] + "@" + row["host"]
		if _, found := identities[identity]; !found {
			continue
		}
		snapshots[identity].GlobalPrivileges.Update(privilegesFromBooleanRow(row, userColumns))
	}

	dbRows, err := client.FetchRows(buildPrivilegeQuery("mysql.db", []string{"User AS user", "Host AS host", "Db AS db_name"}, dbColumns))
	if err != nil {
		return nil, err
	}
	for _, row := range dbRows {
		identity := row["user"] + "@" + row["host"]
		if _, found := identities[identity]; !found {
			continue
		}
		schema := row["db_name"]
		if _, excluded := excludedSchemas[schema]; excluded {
			continue
		}
		privileges := snapshots[identity].DBPrivileges[schema]
		if privileges == nil {
			privileges = StringSet{}
			snapshots[identity].DBPrivileges[schema] = privileges
		}
		privileges.Update(privilegesFromBooleanRow(row, dbColumns))
	}

	tableRows, err := client.FetchRows(`
		SELECT
			User AS user,
			Host AS host,
			Db AS db_name,
			Table_name AS table_name,
			Table_priv AS table_priv
		FROM mysql.tables_priv
		ORDER BY User, Host, Db, Table_name
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range tableRows {
		identity := row["user"] + "@" + row["host"]
		if _, found := identities[identity]; !found {
			continue
		}
		schema := row["db_name"]
		if _, excluded := excludedSchemas[schema]; excluded {
			continue
		}
		scope := TableScope{Schema: schema, Table: row["table_name"]}
		privileges := snapshots[identity].TablePrivileges[scope]
		if privileges == nil {
			privileges = StringSet{}
			snapshots[identity].TablePrivileges[scope] = privileges
		}
		privileges.Update(parsePrivilegeSet(row["table_priv"]))
	}

	result := make([]PrivilegeSnapshot, 0, len(snapshots))
	keys := make([]string, 0, len(snapshots))
	for key := range snapshots {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, *snapshots[key])
	}
	return result, nil
}

func privilegeColumns(client DatabaseClient, tableName string) ([]string, error) {
	rows, err := client.FetchRows("SHOW COLUMNS FROM " + tableName)
	if err != nil {
		return nil, err
	}
	columns := []string{}
	for _, row := range rows {
		if strings.HasSuffix(row["Field"], "_priv") {
			columns = append(columns, row["Field"])
		}
	}
	return columns, nil
}

func buildPrivilegeQuery(tableName string, leadingColumns, privilegeColumns []string) string {
	columns := append([]string{}, leadingColumns...)
	for _, column := range privilegeColumns {
		columns = append(columns, column+" AS "+column)
	}
	return "SELECT " + strings.Join(columns, ", ") + " FROM " + tableName
}

func privilegesFromBooleanRow(row Row, columns []string) []string {
	privileges := []string{}
	for _, column := range columns {
		if row[column] == "Y" {
			privileges = append(privileges, privilegeNameFromColumn(column))
		}
	}
	return privileges
}

func auditInstance(client DatabaseClient, instance string, options Options) (InstanceReport, error) {
	users, err := resolveUsers(client, options.Users, options.ExcludeUsers, options.IncludeAnonymous)
	if err != nil {
		return InstanceReport{}, err
	}
	snapshots, err := collectPrivilegeSnapshots(client, users, options.ExcludeSchemas)
	if err != nil {
		return InstanceReport{}, err
	}
	findings := runRules(instance, snapshots, options.CheckMode)
	return InstanceReport{
		Instance: instance,
		Summary:  buildSummary(snapshots, findings),
		Findings: findings,
	}, nil
}

func buildSummary(snapshots []PrivilegeSnapshot, findings []Finding) AuditSummary {
	userSet := map[string]struct{}{}
	summary := AuditSummary{CheckedIdentities: len(snapshots)}
	for _, snapshot := range snapshots {
		userSet[snapshot.Identity.User] = struct{}{}
	}
	summary.CheckedUsers = len(userSet)
	for _, finding := range findings {
		switch finding.Rule {
		case "inconsistent_host_privileges":
			summary.InconsistentHostPrivilegeUsers++
		case "multi_schema_privileges":
			summary.MultiSchemaIdentities++
		case "db_level_privileges":
			summary.DBLevelPrivilegeIdentities++
		case "table_level_privileges":
			summary.TableLevelPrivilegeIdentities++
		}
		switch finding.Severity {
		case "high":
			summary.HighSeverityCount++
		case "medium":
			summary.MediumSeverityCount++
		default:
			summary.LowSeverityCount++
		}
	}
	return summary
}

func identitySnapshotByHost(snapshot PrivilegeSnapshot) map[string]any {
	return map[string]any{
		"host":              snapshot.Identity.Host,
		"global_privileges": snapshot.GlobalPrivileges.Sorted(),
		"db_privileges":     dbPrivilegesToMap(snapshot.DBPrivileges),
		"table_privileges":  tablePrivilegesToMap(snapshot.TablePrivileges),
	}
}

func dbPrivilegesToMap(values map[string]StringSet) map[string][]string {
	result := map[string][]string{}
	for key, privileges := range values {
		result[key] = privileges.Sorted()
	}
	return result
}

func tablePrivilegesToMap(values map[TableScope]StringSet) map[string][]string {
	result := map[string][]string{}
	for key, privileges := range values {
		result[key.DisplayName()] = privileges.Sorted()
	}
	return result
}

func snapshotSignature(snapshot PrivilegeSnapshot) string {
	return fmt.Sprint(snapshot.ToMap())
}
