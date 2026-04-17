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
	lines := []string{
		"Summary:",
		"  Total targets: " + itoa(summary.TotalTargets),
		"  Successful targets: " + itoa(summary.SuccessfulTargets),
		"  Failed targets: " + itoa(summary.FailedTargets),
		"  Consistent targets: " + itoa(summary.ConsistentTargets),
		"  Inconsistent targets: " + itoa(summary.InconsistentTargets),
		"  Exit code: " + itoa(exitCode),
	}
	if len(summary.FailedTargetDetails) > 0 {
		lines = append(lines, "  Failed target details:")
		for _, detail := range summary.FailedTargetDetails {
			lines = append(lines, renderTargetSummaryDetail("    - ", detail)...)
		}
	}
	if len(summary.InconsistentDetails) > 0 {
		lines = append(lines, "  Inconsistent target details:")
		for _, detail := range summary.InconsistentDetails {
			lines = append(lines, renderTargetSummaryDetail("    - ", detail)...)
		}
	}
	return lines
}

func renderTargetSummaryDetail(prefix string, detail TargetSummaryDetail) []string {
	parts := []string{
		prefix + "target=" + detail.Target,
		"      host=" + detail.Host,
		"      port=" + itoa(detail.Port),
	}
	if detail.Database != "" {
		parts = append(parts, "      database="+detail.Database)
	}
	if len(detail.ComparedSchemas) > 0 {
		parts = append(parts, "      compared_schemas="+strings.Join(detail.ComparedSchemas, ","))
	}
	if detail.Error != "" {
		parts = append(parts, "      error="+detail.Error)
	}
	return parts
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
		detailBlocks = append(detailBlocks, []string{
			"      source only table: " + tableName,
			"        reason: table exists in source but is missing in target",
		})
	}
	for _, tableName := range schemaDiff.TargetOnlyTables {
		detailBlocks = append(detailBlocks, []string{
			"      target only table: " + tableName,
			"        reason: table exists in target but is missing in source",
		})
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
		lines = append(lines,
			"        option "+option+": source="+toString(values["source"])+" target="+toString(values["target"]),
			"          reason: table option definition is different between source and target",
		)
	}
	for _, column := range tableDiff.SourceOnlyColumns {
		lines = append(lines,
			"        source only column: "+toString(column["name"]),
			"          reason: column exists in source but is missing in target",
		)
	}
	for _, column := range tableDiff.TargetOnlyColumns {
		lines = append(lines,
			"        target only column: "+toString(column["name"]),
			"          reason: column exists in target but is missing in source",
		)
	}
	for _, column := range tableDiff.ChangedColumns {
		sourceDiff, targetDiff := diffPayloadFields(column["source"], column["target"])
		sourcePayload, _ := json.Marshal(sourceDiff)
		targetPayload, _ := json.Marshal(targetDiff)
		lines = append(lines,
			"        changed column: "+toString(column["column"]),
			"          reason: column definition is different between source and target",
			"          source="+string(sourcePayload),
			"          target="+string(targetPayload),
		)
	}
	for _, index := range tableDiff.SourceOnlyIndexes {
		lines = append(lines,
			"        source only index: "+toString(index["name"]),
			"          reason: index exists in source but is missing in target",
		)
	}
	for _, index := range tableDiff.TargetOnlyIndexes {
		lines = append(lines,
			"        target only index: "+toString(index["name"]),
			"          reason: index exists in target but is missing in source",
		)
	}
	for _, index := range tableDiff.ChangedIndexes {
		sourceDiff, targetDiff := diffPayloadFields(index["source"], index["target"])
		sourcePayload, _ := json.Marshal(sourceDiff)
		targetPayload, _ := json.Marshal(targetDiff)
		lines = append(lines,
			"        changed index: "+toString(index["index"]),
			"          reason: index definition is different between source and target",
			"          source="+string(sourcePayload),
			"          target="+string(targetPayload),
		)
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
		sourceDiff, targetDiff := diffPayloadFields(item["source"], item["target"])
		sourcePayload, _ := json.Marshal(sourceDiff)
		targetPayload, _ := json.Marshal(targetDiff)
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

func diffPayloadFields(source, target any) (map[string]any, map[string]any) {
	sourceValue, targetValue, changed := diffPayloadValue(source, target)
	if !changed {
		return map[string]any{}, map[string]any{}
	}

	sourceMap, sourceOK := sourceValue.(map[string]any)
	targetMap, targetOK := targetValue.(map[string]any)
	if sourceOK && targetOK {
		return sourceMap, targetMap
	}
	return map[string]any{"value": sourceValue}, map[string]any{"value": targetValue}
}

func diffPayloadValue(source, target any) (any, any, bool) {
	sourceMap, sourceOK := source.(map[string]any)
	targetMap, targetOK := target.(map[string]any)
	if sourceOK && targetOK {
		sourceDiff := map[string]any{}
		targetDiff := map[string]any{}
		for _, key := range unionKeys(sourceMap, targetMap) {
			sourceValue, sourceFound := sourceMap[key]
			targetValue, targetFound := targetMap[key]
			if !sourceFound || !targetFound {
				sourceDiff[key] = sourceValue
				targetDiff[key] = targetValue
				continue
			}
			left, right, changed := diffPayloadValue(sourceValue, targetValue)
			if changed {
				sourceDiff[key] = left
				targetDiff[key] = right
			}
		}
		if len(sourceDiff) == 0 && len(targetDiff) == 0 {
			return nil, nil, false
		}
		return sourceDiff, targetDiff, true
	}

	if payloadValueEqual(source, target) {
		return nil, nil, false
	}
	return source, target, true
}

func unionKeys(left, right map[string]any) []string {
	keySet := map[string]struct{}{}
	for key := range left {
		keySet[key] = struct{}{}
	}
	for key := range right {
		keySet[key] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	return uniqSorted(keys)
}

func payloadValueEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return toString(left) == toString(right)
	}
	return string(leftJSON) == string(rightJSON)
}
