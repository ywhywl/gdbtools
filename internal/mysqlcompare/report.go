package mysqlcompare

import (
	"encoding/json"
	"strings"
)

func renderTargetReport(comparison TargetComparison, outputFormat string) string {
	if outputFormat == "json" {
		payload := map[string]any{
			"target":          comparison.Target,
			"status":          map[bool]string{true: "failed", false: "success"}[comparison.Error != ""],
			"error":           emptyStringAsNil(comparison.Error),
			"has_differences": comparison.HasDifferences(),
		}
		if comparison.IncludeStructure {
			payload["schema_pairs"] = comparison.SchemaPairs
			payload["schema_diffs"] = comparison.SchemaDiffs
		}
		if comparison.IncludePrivileges {
			payload["privilege_diff"] = comparison.PrivilegeDiff
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		return string(data)
	}
	return strings.Join(renderTextTarget(comparison), "\n")
}

func renderSummaryReport(summary ComparisonSummary, outputFormat string, exitCode int) string {
	if outputFormat == "json" {
		data, _ := json.MarshalIndent(map[string]any{
			"summary":   summary,
			"exit_code": exitCode,
		}, "", "  ")
		return string(data)
	}
	return strings.Join(renderTextSummary(summary, exitCode), "\n")
}

func renderTextTarget(comparison TargetComparison) []string {
	lines := []string{"Target: " + comparison.Target}
	if comparison.Error != "" {
		lines = append(lines, "  Status: failed", "  Error: "+comparison.Error)
		return lines
	}
	status := "consistent"
	if comparison.HasDifferences() {
		status = "inconsistent"
	}
	lines = append(lines, "  Status: "+status)

	if comparison.IncludeStructure {
		if len(comparison.SchemaPairs) == 0 {
			lines = append(lines, "  Schema pairs: none")
		} else {
			lines = append(lines, "  Schema pairs:")
			for _, pair := range comparison.SchemaPairs {
				lines = append(lines, "    - "+pair.SourceSchema+" -> "+pair.TargetSchema)
			}
		}
		if len(comparison.SchemaDiffs) == 0 {
			lines = append(lines, "  Structure diff: no differences")
		} else {
			lines = append(lines, "  Structure diff:")
			for _, schemaDiff := range comparison.SchemaDiffs {
				lines = append(lines, renderSchemaDiff(schemaDiff)...)
			}
		}
	}

	if comparison.IncludePrivileges {
		if !comparison.PrivilegeDiff.HasChanges() {
			lines = append(lines, "  Privilege diff: no differences")
		} else {
			lines = append(lines, "  Privilege diff:")
			lines = append(lines, renderPrivilegeDiff(comparison.PrivilegeDiff)...)
		}
	}
	return lines
}

func renderTextSummary(summary ComparisonSummary, exitCode int) []string {
	return []string{
		"Summary:",
		"  Total targets: " + itoa(summary.TotalTargets),
		"  Successful targets: " + itoa(summary.SuccessfulTargets),
		"  Failed targets: " + itoa(summary.FailedTargets),
		"  Consistent targets: " + itoa(summary.ConsistentTargets),
		"  Inconsistent targets: " + itoa(summary.InconsistentTargets),
		"  Exit code: " + itoa(exitCode),
	}
}

func renderSchemaDiff(schemaDiff SchemaDiff) []string {
	lines := []string{"    " + schemaDiff.SourceSchema + " -> " + schemaDiff.TargetSchema}
	if !schemaDiff.HasChanges() {
		return append(lines, "      no differences")
	}
	total := schemaDiff.TableDifferenceCount()
	lines = append(lines, "      table differences: total="+itoa(total)+limitSuffix(total))

	detailBlocks := [][]string{}
	for _, tableName := range schemaDiff.SourceOnlyTables {
		detailBlocks = append(detailBlocks, []string{"      source only table: " + tableName})
	}
	for _, tableName := range schemaDiff.TargetOnlyTables {
		detailBlocks = append(detailBlocks, []string{"      target only table: " + tableName})
	}
	for _, tableDiff := range schemaDiff.ChangedTables {
		detailBlocks = append(detailBlocks, renderTableDiff(tableDiff))
	}
	for _, block := range detailBlocks[:min(len(detailBlocks), diffDetailLimit)] {
		lines = append(lines, block...)
	}
	if total > diffDetailLimit {
		lines = append(lines, "      omitted table detail count: "+itoa(total-diffDetailLimit))
	}
	return lines
}

func renderTableDiff(tableDiff TableDiff) []string {
	lines := []string{"      table: " + tableDiff.Table}
	for _, option := range mapKeys(tableDiff.ChangedTableOptions) {
		values := tableDiff.ChangedTableOptions[option]
		lines = append(lines, "        option "+option+": source="+toString(values["source"])+" target="+toString(values["target"]))
	}
	for _, column := range tableDiff.SourceOnlyColumns {
		lines = append(lines, "        source only column: "+toString(column["name"]))
	}
	for _, column := range tableDiff.TargetOnlyColumns {
		lines = append(lines, "        target only column: "+toString(column["name"]))
	}
	for _, column := range tableDiff.ChangedColumns {
		lines = append(lines, "        changed column: "+toString(column["column"]))
	}
	for _, index := range tableDiff.SourceOnlyIndexes {
		lines = append(lines, "        source only index: "+toString(index["name"]))
	}
	for _, index := range tableDiff.TargetOnlyIndexes {
		lines = append(lines, "        target only index: "+toString(index["name"]))
	}
	for _, index := range tableDiff.ChangedIndexes {
		lines = append(lines, "        changed index: "+toString(index["index"]))
	}
	return lines
}

func renderPrivilegeDiff(privilegeDiff PrivilegeDiff) []string {
	total := privilegeDiff.DifferenceCount()
	lines := []string{"    privilege differences: total=" + itoa(total) + limitSuffix(total)}
	entries := [][]string{}
	for _, identity := range privilegeDiff.SourceOnlyIdentities {
		entries = append(entries, []string{"    source only identity: " + toString(identity["identity"])})
	}
	for _, identity := range privilegeDiff.TargetOnlyIdentities {
		entries = append(entries, []string{"    target only identity: " + toString(identity["identity"])})
	}
	for _, item := range privilegeDiff.ChangedIdentities {
		sourcePayload, _ := json.Marshal(item["source"])
		targetPayload, _ := json.Marshal(item["target"])
		entries = append(entries, []string{
			"    changed identity: " + toString(item["identity"]),
			"      source=" + string(sourcePayload),
			"      target=" + string(targetPayload),
		})
	}
	for _, entry := range entries[:min(len(entries), diffDetailLimit)] {
		lines = append(lines, entry...)
	}
	if total > diffDetailLimit {
		lines = append(lines, "    omitted privilege detail count: "+itoa(total-diffDetailLimit))
	}
	return lines
}

func limitSuffix(total int) string {
	if total > diffDetailLimit {
		return ", showing_first=" + itoa(diffDetailLimit)
	}
	return ""
}

func emptyStringAsNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
