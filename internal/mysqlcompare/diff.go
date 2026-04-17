package mysqlcompare

import "reflect"

func mapSchemaPairs(
	sourceAvailable, sourceSelected, sourceSelectors, targetAvailable, targetSelected, targetSelectors []string,
) ([]SchemaPair, error) {
	if len(sourceSelected) == 0 || len(targetSelected) == 0 {
		return nil, nil
	}
	sourceExact := len(sourceSelectors) > 0
	for _, selector := range sourceSelectors {
		if !contains(sourceAvailable, selector) {
			sourceExact = false
			break
		}
	}
	targetExact := len(targetSelectors) > 0
	for _, selector := range targetSelectors {
		if !contains(targetAvailable, selector) {
			targetExact = false
			break
		}
	}

	if sourceExact && targetExact {
		if len(sourceSelected) == 1 {
			pairs := make([]SchemaPair, 0, len(targetSelected))
			for _, target := range targetSelected {
				pairs = append(pairs, SchemaPair{SourceSchema: sourceSelected[0], TargetSchema: target})
			}
			return pairs, nil
		}
		if len(targetSelected) == 1 {
			pairs := make([]SchemaPair, 0, len(sourceSelected))
			for _, source := range sourceSelected {
				pairs = append(pairs, SchemaPair{SourceSchema: source, TargetSchema: targetSelected[0]})
			}
			return pairs, nil
		}
		if len(sourceSelected) == len(targetSelected) {
			pairs := make([]SchemaPair, 0, len(sourceSelected))
			for index := range sourceSelected {
				pairs = append(pairs, SchemaPair{SourceSchema: sourceSelected[index], TargetSchema: targetSelected[index]})
			}
			return pairs, nil
		}
		return nil, newUsageError("exact source and target schema counts are different and cannot be paired")
	}

	if sourceExact && len(sourceSelected) == 1 {
		pairs := make([]SchemaPair, 0, len(targetSelected))
		for _, target := range targetSelected {
			pairs = append(pairs, SchemaPair{SourceSchema: sourceSelected[0], TargetSchema: target})
		}
		return pairs, nil
	}

	if targetExact && len(targetSelected) == 1 {
		pairs := make([]SchemaPair, 0, len(sourceSelected))
		for _, source := range sourceSelected {
			pairs = append(pairs, SchemaPair{SourceSchema: source, TargetSchema: targetSelected[0]})
		}
		return pairs, nil
	}

	common := []SchemaPair{}
	for _, name := range sourceSelected {
		if contains(targetSelected, name) {
			common = append(common, SchemaPair{SourceSchema: name, TargetSchema: name})
		}
	}
	return common, nil
}

func diffSchema(source, target SchemaSnapshot) SchemaDiff {
	sourceTables := source.Tables
	targetTables := target.Tables
	sourceOnlyTables := []string{}
	targetOnlyTables := []string{}
	changedTables := []TableDiff{}

	for tableName := range sourceTables {
		if _, found := targetTables[tableName]; !found {
			sourceOnlyTables = append(sourceOnlyTables, tableName)
		}
	}
	for tableName := range targetTables {
		if _, found := sourceTables[tableName]; !found {
			targetOnlyTables = append(targetOnlyTables, tableName)
		}
	}
	sourceOnlyTables = uniqSorted(sourceOnlyTables)
	targetOnlyTables = uniqSorted(targetOnlyTables)

	for _, tableName := range intersectKeys(sourceTables, targetTables) {
		sourceTable := sourceTables[tableName]
		targetTable := targetTables[tableName]
		tableDiff := TableDiff{
			Table:               tableName,
			ChangedTableOptions: map[string]map[string]any{},
		}

		if valueString(sourceTable.Engine) != valueString(targetTable.Engine) {
			tableDiff.ChangedTableOptions["engine"] = map[string]any{"source": valuePtr(sourceTable.Engine), "target": valuePtr(targetTable.Engine)}
		}
		if valueString(sourceTable.RowFormat) != valueString(targetTable.RowFormat) {
			tableDiff.ChangedTableOptions["row_format"] = map[string]any{"source": valuePtr(sourceTable.RowFormat), "target": valuePtr(targetTable.RowFormat)}
		}
		if valueString(sourceTable.TableCollation) != valueString(targetTable.TableCollation) {
			tableDiff.ChangedTableOptions["table_collation"] = map[string]any{"source": valuePtr(sourceTable.TableCollation), "target": valuePtr(targetTable.TableCollation)}
		}
		if valueString(sourceTable.CreateOptions) != valueString(targetTable.CreateOptions) {
			tableDiff.ChangedTableOptions["create_options"] = map[string]any{"source": valuePtr(sourceTable.CreateOptions), "target": valuePtr(targetTable.CreateOptions)}
		}
		if valueString(sourceTable.TableComment) != valueString(targetTable.TableComment) {
			tableDiff.ChangedTableOptions["table_comment"] = map[string]any{"source": valuePtr(sourceTable.TableComment), "target": valuePtr(targetTable.TableComment)}
		}

		sourceColumns := map[string]ColumnMeta{}
		targetColumns := map[string]ColumnMeta{}
		for _, column := range sourceTable.Columns {
			sourceColumns[column.Name] = column
		}
		for _, column := range targetTable.Columns {
			targetColumns[column.Name] = column
		}
		for _, columnName := range diffKeys(sourceColumns, targetColumns) {
			tableDiff.SourceOnlyColumns = append(tableDiff.SourceOnlyColumns, columnToMap(sourceColumns[columnName]))
		}
		for _, columnName := range diffKeys(targetColumns, sourceColumns) {
			tableDiff.TargetOnlyColumns = append(tableDiff.TargetOnlyColumns, columnToMap(targetColumns[columnName]))
		}
		for _, columnName := range intersectKeys(sourceColumns, targetColumns) {
			sourceColumn := columnToMap(sourceColumns[columnName])
			targetColumn := columnToMap(targetColumns[columnName])
			if !reflect.DeepEqual(sourceColumn, targetColumn) {
				tableDiff.ChangedColumns = append(tableDiff.ChangedColumns, map[string]any{
					"column": columnName,
					"source": sourceColumn,
					"target": targetColumn,
				})
			}
		}

		sourceIndexes := map[string]map[string]any{}
		targetIndexes := map[string]map[string]any{}
		for _, index := range sourceTable.Indexes {
			sourceIndexes[index.Name] = indexToMap(index)
		}
		for _, index := range targetTable.Indexes {
			targetIndexes[index.Name] = indexToMap(index)
		}
		for _, indexName := range diffKeys(sourceIndexes, targetIndexes) {
			tableDiff.SourceOnlyIndexes = append(tableDiff.SourceOnlyIndexes, sourceIndexes[indexName])
		}
		for _, indexName := range diffKeys(targetIndexes, sourceIndexes) {
			tableDiff.TargetOnlyIndexes = append(tableDiff.TargetOnlyIndexes, targetIndexes[indexName])
		}
		for _, indexName := range intersectKeys(sourceIndexes, targetIndexes) {
			if !reflect.DeepEqual(sourceIndexes[indexName], targetIndexes[indexName]) {
				tableDiff.ChangedIndexes = append(tableDiff.ChangedIndexes, map[string]any{
					"index":  indexName,
					"source": sourceIndexes[indexName],
					"target": targetIndexes[indexName],
				})
			}
		}

		if tableDiff.HasChanges() {
			changedTables = append(changedTables, tableDiff)
		}
	}

	return SchemaDiff{
		SourceSchema:     source.Name,
		TargetSchema:     target.Name,
		SourceOnlyTables: sourceOnlyTables,
		TargetOnlyTables: targetOnlyTables,
		ChangedTables:    changedTables,
	}
}

func diffPrivileges(sourceBundles, targetBundles map[string]*PrivilegeBundle) PrivilegeDiff {
	sourceKeys := mapKeys(sourceBundles)
	targetKeys := mapKeys(targetBundles)
	sourceOnly := []map[string]any{}
	targetOnly := []map[string]any{}
	changed := []map[string]any{}

	for _, key := range diffStrings(sourceKeys, targetKeys) {
		sourceOnly = append(sourceOnly, sourceBundles[key].ToMap())
	}
	for _, key := range diffStrings(targetKeys, sourceKeys) {
		targetOnly = append(targetOnly, targetBundles[key].ToMap())
	}
	for _, key := range intersectStrings(sourceKeys, targetKeys) {
		sourcePayload := sourceBundles[key].ToMap()
		targetPayload := targetBundles[key].ToMap()
		if !reflect.DeepEqual(sourcePayload, targetPayload) {
			changed = append(changed, map[string]any{
				"identity": sourceBundles[key].Identity.DisplayName(),
				"source":   sourcePayload,
				"target":   targetPayload,
			})
		}
	}

	return PrivilegeDiff{
		SourceOnlyIdentities: sourceOnly,
		TargetOnlyIdentities: targetOnly,
		ChangedIdentities:    changed,
	}
}

func columnToMap(column ColumnMeta) map[string]any {
	return map[string]any{
		"ordinal_position":   column.OrdinalPosition,
		"name":               column.Name,
		"column_type":        column.ColumnType,
		"is_nullable":        column.IsNullable,
		"column_default":     valuePtr(column.ColumnDefault),
		"extra":              column.Extra,
		"character_set_name": valuePtr(column.CharacterSetName),
		"collation_name":     valuePtr(column.CollationName),
		"column_comment":     column.ColumnComment,
	}
}

func indexToMap(index IndexMeta) map[string]any {
	columns := make([]map[string]any, 0, len(index.Columns))
	for _, column := range index.Columns {
		columns = append(columns, map[string]any{
			"seq_in_index": column.SeqInIndex,
			"column_name":  column.ColumnName,
			"collation":    valuePtr(column.Collation),
			"sub_part":     valueIntPtr(column.SubPart),
			"nullable":     valueBoolPtr(column.Nullable),
		})
	}
	return map[string]any{
		"name":       index.Name,
		"non_unique": index.NonUnique,
		"index_type": index.IndexType,
		"columns":    columns,
	}
}

func mapKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return uniqSorted(keys)
}

func diffKeys[V any, W any](left map[string]V, right map[string]W) []string {
	items := []string{}
	for key := range left {
		if _, found := right[key]; !found {
			items = append(items, key)
		}
	}
	return uniqSorted(items)
}

func intersectKeys[V any, W any](left map[string]V, right map[string]W) []string {
	items := []string{}
	for key := range left {
		if _, found := right[key]; found {
			items = append(items, key)
		}
	}
	return uniqSorted(items)
}

func diffStrings(left, right []string) []string {
	rightSet := map[string]struct{}{}
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	items := []string{}
	for _, item := range left {
		if _, found := rightSet[item]; !found {
			items = append(items, item)
		}
	}
	return uniqSorted(items)
}

func intersectStrings(left, right []string) []string {
	rightSet := map[string]struct{}{}
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	items := []string{}
	for _, item := range left {
		if _, found := rightSet[item]; found {
			items = append(items, item)
		}
	}
	return uniqSorted(items)
}

func valuePtr(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func valueIntPtr(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func valueBoolPtr(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
