package mysqlpricheck

import (
	"sort"
)

func runRules(instance string, snapshots []PrivilegeSnapshot, checkMode string) []Finding {
	findings := []Finding{}
	if checkMode == "all" || checkMode == "host_consistency" {
		findings = append(findings, checkInconsistentHostPrivileges(instance, snapshots)...)
	}
	if checkMode == "all" || checkMode == "multi_schema" {
		findings = append(findings, checkMultiSchemaPrivileges(instance, snapshots)...)
	}
	if checkMode == "all" || checkMode == "db_level" {
		findings = append(findings, checkDBLevelPrivileges(instance, snapshots)...)
	}
	if checkMode == "all" || checkMode == "table_level" {
		findings = append(findings, checkTableLevelPrivileges(instance, snapshots)...)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		left, right := findings[i], findings[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) < severityRank(right.Severity)
		}
		if left.Rule != right.Rule {
			return left.Rule < right.Rule
		}
		if left.User != right.User {
			return left.User < right.User
		}
		return left.Identity < right.Identity
	})
	return findings
}

func checkInconsistentHostPrivileges(instance string, snapshots []PrivilegeSnapshot) []Finding {
	grouped := map[string][]PrivilegeSnapshot{}
	for _, snapshot := range snapshots {
		grouped[snapshot.Identity.User] = append(grouped[snapshot.Identity.User], snapshot)
	}
	findings := []Finding{}
	for user, items := range grouped {
		if len(items) < 2 {
			continue
		}
		signatures := map[string]struct{}{}
		hostSnapshots := make([]map[string]any, 0, len(items))
		hosts := make([]string, 0, len(items))
		for _, item := range items {
			signatures[snapshotSignature(item)] = struct{}{}
			hostSnapshots = append(hostSnapshots, identitySnapshotByHost(item))
			hosts = append(hosts, item.Identity.Host)
		}
		if len(signatures) <= 1 {
			continue
		}
		sort.Strings(hosts)
		findings = append(findings, Finding{
			Rule:     "inconsistent_host_privileges",
			Severity: "high",
			Instance: instance,
			User:     user,
			Summary:  user + " has different privileges across hosts",
			Details: map[string]any{
				"hosts":     uniqSorted(hosts),
				"snapshots": hostSnapshots,
			},
		})
	}
	return findings
}

func checkMultiSchemaPrivileges(instance string, snapshots []PrivilegeSnapshot) []Finding {
	findings := []Finding{}
	grouped := groupSnapshotsByUser(snapshots)
	for user, items := range grouped {
		schemas := schemasForSnapshots(items)
		if len(schemas) <= 1 {
			continue
		}
		findings = append(findings, Finding{
			Rule:     "multi_schema_privileges",
			Severity: "medium",
			Instance: instance,
			User:     user,
			Summary:  user + " has privileges on multiple schemas",
			Details: map[string]any{
				"schemas":   schemas,
				"snapshots": identitySnapshotsByHost(items),
			},
		})
	}
	return findings
}

func checkDBLevelPrivileges(instance string, snapshots []PrivilegeSnapshot) []Finding {
	findings := []Finding{}
	grouped := groupSnapshotsByUser(snapshots)
	for user, items := range grouped {
		schemas := schemasForSnapshots(items)
		if len(schemas) == 0 {
			continue
		}
		findings = append(findings, Finding{
			Rule:     "db_level_privileges",
			Severity: "medium",
			Instance: instance,
			User:     user,
			Summary:  user + " has database-level privileges",
			Details: map[string]any{
				"schemas":   schemas,
				"snapshots": identitySnapshotsByHost(items),
			},
		})
	}
	return findings
}

func checkTableLevelPrivileges(instance string, snapshots []PrivilegeSnapshot) []Finding {
	findings := []Finding{}
	grouped := groupSnapshotsByUser(snapshots)
	for user, items := range grouped {
		if !hasTablePrivileges(items) {
			continue
		}
		findings = append(findings, Finding{
			Rule:     "table_level_privileges",
			Severity: "medium",
			Instance: instance,
			User:     user,
			Summary:  user + " has table-level privileges",
			Details: map[string]any{
				"snapshots": identitySnapshotsByHost(items),
			},
		})
	}
	return findings
}

func groupSnapshotsByUser(snapshots []PrivilegeSnapshot) map[string][]PrivilegeSnapshot {
	grouped := map[string][]PrivilegeSnapshot{}
	for _, snapshot := range snapshots {
		grouped[snapshot.Identity.User] = append(grouped[snapshot.Identity.User], snapshot)
	}
	return grouped
}

func schemasForSnapshots(snapshots []PrivilegeSnapshot) []string {
	schemas := map[string]struct{}{}
	for _, snapshot := range snapshots {
		for schema := range snapshot.DBPrivileges {
			schemas[schema] = struct{}{}
		}
	}
	return sortedStringKeys(schemas)
}

func hasTablePrivileges(snapshots []PrivilegeSnapshot) bool {
	for _, snapshot := range snapshots {
		if len(snapshot.TablePrivileges) > 0 {
			return true
		}
	}
	return false
}

func identitySnapshotsByHost(snapshots []PrivilegeSnapshot) []map[string]any {
	items := make([]PrivilegeSnapshot, len(snapshots))
	copy(items, snapshots)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Identity.Host < items[j].Identity.Host
	})
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, identitySnapshotByHost(item))
	}
	return result
}

func severityRank(value string) int {
	switch value {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func sortedStringKeys(items map[string]struct{}) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
