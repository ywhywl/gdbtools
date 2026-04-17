package mysqlcompare

import (
	"fmt"
	"sort"
	"strings"
)

func listSchemas(client DatabaseClient) ([]string, error) {
	rows, err := client.FetchRows(`
		SELECT SCHEMA_NAME AS schema_name
		FROM information_schema.SCHEMATA
		ORDER BY SCHEMA_NAME
	`)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		name := row["schema_name"]
		if _, found := systemSchemas[name]; found {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func resolveSchemas(client DatabaseClient, selectors, excludeSelectors []string) ([]string, error) {
	available, err := listSchemas(client)
	if err != nil {
		return nil, err
	}
	return filterNames(available, selectors, excludeSelectors), nil
}

func collectSchemaSnapshot(client DatabaseClient, schemaName string) (SchemaSnapshot, error) {
	tableRows, err := client.FetchRows(`
		SELECT
			TABLE_NAME AS table_name,
			ENGINE AS engine,
			ROW_FORMAT AS row_format,
			TABLE_COLLATION AS table_collation,
			CREATE_OPTIONS AS create_options,
			TABLE_COMMENT AS table_comment
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME
	`, schemaName)
	if err != nil {
		return SchemaSnapshot{}, err
	}
	columnRows, err := client.FetchRows(`
		SELECT
			TABLE_NAME AS table_name,
			ORDINAL_POSITION AS ordinal_position,
			COLUMN_NAME AS column_name,
			COLUMN_TYPE AS column_type,
			IS_NULLABLE AS is_nullable,
			COLUMN_DEFAULT AS column_default,
			EXTRA AS extra,
			CHARACTER_SET_NAME AS character_set_name,
			COLLATION_NAME AS collation_name,
			COLUMN_COMMENT AS column_comment
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, ORDINAL_POSITION
	`, schemaName)
	if err != nil {
		return SchemaSnapshot{}, err
	}
	indexRows, err := client.FetchRows(`
		SELECT
			TABLE_NAME AS table_name,
			INDEX_NAME AS index_name,
			NON_UNIQUE AS non_unique,
			SEQ_IN_INDEX AS seq_in_index,
			COLUMN_NAME AS column_name,
			COLLATION AS collation,
			SUB_PART AS sub_part,
			NULLABLE AS nullable,
			INDEX_TYPE AS index_type
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX
	`, schemaName)
	if err != nil {
		return SchemaSnapshot{}, err
	}

	columnsByTable := map[string][]ColumnMeta{}
	for _, row := range columnRows {
		columnsByTable[row["table_name"]] = append(columnsByTable[row["table_name"]], ColumnMeta{
			OrdinalPosition:  mustAtoi(row["ordinal_position"]),
			Name:             row["column_name"],
			ColumnType:       row["column_type"],
			IsNullable:       row["is_nullable"] == "YES",
			ColumnDefault:    parseNullableString(row["column_default"]),
			Extra:            normalizeExtra(row["extra"]),
			CharacterSetName: parseNullableString(row["character_set_name"]),
			CollationName:    parseNullableString(row["collation_name"]),
			ColumnComment:    row["column_comment"],
		})
	}

	type rawIndex struct {
		Name      string
		NonUnique bool
		IndexType string
		Columns   []IndexColumnMeta
	}
	indexesByTable := map[string]map[string]*rawIndex{}
	for _, row := range indexRows {
		tableIndexes := indexesByTable[row["table_name"]]
		if tableIndexes == nil {
			tableIndexes = map[string]*rawIndex{}
			indexesByTable[row["table_name"]] = tableIndexes
		}
		payload := tableIndexes[row["index_name"]]
		if payload == nil {
			payload = &rawIndex{
				Name:      row["index_name"],
				NonUnique: row["non_unique"] == "1",
				IndexType: row["index_type"],
			}
			tableIndexes[row["index_name"]] = payload
		}
		payload.Columns = append(payload.Columns, IndexColumnMeta{
			SeqInIndex: mustAtoi(row["seq_in_index"]),
			ColumnName: row["column_name"],
			Collation:  parseNullableString(row["collation"]),
			SubPart:    parseNullableInt(row["sub_part"]),
			Nullable:   parseNullableBool(row["nullable"]),
		})
	}

	tables := map[string]TableMeta{}
	for _, row := range tableRows {
		tableName := row["table_name"]
		indexes := []IndexMeta{}
		if rawIndexes := indexesByTable[tableName]; rawIndexes != nil {
			indexNames := make([]string, 0, len(rawIndexes))
			for name := range rawIndexes {
				indexNames = append(indexNames, name)
			}
			sort.Strings(indexNames)
			for _, name := range indexNames {
				indexes = append(indexes, IndexMeta{
					Name:      rawIndexes[name].Name,
					NonUnique: rawIndexes[name].NonUnique,
					IndexType: rawIndexes[name].IndexType,
					Columns:   rawIndexes[name].Columns,
				})
			}
		}
		tables[tableName] = TableMeta{
			Name:           tableName,
			Engine:         parseNullableString(row["engine"]),
			RowFormat:      parseNullableString(row["row_format"]),
			TableCollation: parseNullableString(row["table_collation"]),
			CreateOptions:  parseNullableNormalizedString(row["create_options"]),
			TableComment:   parseNullableString(row["table_comment"]),
			Columns:        columnsByTable[tableName],
			Indexes:        indexes,
		}
	}
	return SchemaSnapshot{Name: schemaName, Tables: tables}, nil
}

func resolveUsers(client DatabaseClient, selectors, excludeSelectors []string) ([][2]string, error) {
	rows, err := client.FetchRows(`
		SELECT User AS user, Host AS host
		FROM mysql.user
		ORDER BY User, Host
	`)
	if err != nil {
		return nil, err
	}
	identities := make([]string, 0, len(rows))
	userToIdentities := map[string][]string{}
	for _, row := range rows {
		identity := row["user"] + "@" + row["host"]
		identities = append(identities, identity)
		userToIdentities[row["user"]] = append(userToIdentities[row["user"]], identity)
	}

	selected := map[string]struct{}{}
	if len(selectors) == 0 {
		for _, identity := range identities {
			selected[identity] = struct{}{}
		}
	} else {
		for _, selector := range selectors {
			if strings.Contains(selector, "@") {
				if contains(identities, selector) {
					selected[selector] = struct{}{}
					continue
				}
				for _, identity := range identities {
					if matchesSelector(identity, selector) {
						selected[identity] = struct{}{}
					}
				}
				continue
			}
			if exactMatches, found := userToIdentities[selector]; found {
				for _, identity := range exactMatches {
					selected[identity] = struct{}{}
				}
				continue
			}
			for userName, matchedIdentities := range userToIdentities {
				if matchesSelector(userName, selector) {
					for _, identity := range matchedIdentities {
						selected[identity] = struct{}{}
					}
				}
			}
		}
	}

	excluded := map[string]struct{}{}
	for _, selector := range excludeSelectors {
		if strings.Contains(selector, "@") {
			if contains(identities, selector) {
				excluded[selector] = struct{}{}
			} else {
				for _, identity := range identities {
					if matchesSelector(identity, selector) {
						excluded[identity] = struct{}{}
					}
				}
			}
			continue
		}
		if exactMatches, found := userToIdentities[selector]; found {
			for _, identity := range exactMatches {
				excluded[identity] = struct{}{}
			}
			continue
		}
		for userName, matchedIdentities := range userToIdentities {
			if matchesSelector(userName, selector) {
				for _, identity := range matchedIdentities {
					excluded[identity] = struct{}{}
				}
			}
		}
	}

	keys := make([]string, 0, len(selected))
	for identity := range selected {
		if _, found := excluded[identity]; !found {
			keys = append(keys, identity)
		}
	}
	sort.Strings(keys)
	result := make([][2]string, 0, len(keys))
	for _, identity := range keys {
		user, host, found := strings.Cut(identity, "@")
		if !found {
			return nil, fmt.Errorf("invalid user identity: %s", identity)
		}
		result = append(result, [2]string{user, host})
	}
	return result, nil
}

func collectPrivileges(client DatabaseClient, users [][2]string, matchMode string, schemaScope []string) (map[string]*PrivilegeBundle, error) {
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
		identities[item[0]+"@"+item[1]] = struct{}{}
	}
	schemaScopeSet := map[string]struct{}{}
	for _, schemaName := range schemaScope {
		schemaScopeSet[schemaName] = struct{}{}
	}

	bundles := map[string]*PrivilegeBundle{}

	globalRows, err := client.FetchRows(buildPrivilegeQuery("mysql.user", []string{"User AS user", "Host AS host"}, userColumns))
	if err != nil {
		return nil, err
	}
	for _, row := range globalRows {
		if !hasIdentity(identities, row["user"], row["host"]) {
			continue
		}
		bundle := ensureBundle(bundles, row["user"], row["host"], matchMode)
		bundle.Hosts.Add(row["host"])
		bundle.GlobalPrivileges.Update(privilegesFromBooleanRow(row, userColumns))
	}

	dbRows, err := client.FetchRows(buildPrivilegeQuery("mysql.db", []string{"User AS user", "Host AS host", "Db AS db_name"}, dbColumns))
	if err != nil {
		return nil, err
	}
	for _, row := range dbRows {
		if !hasIdentity(identities, row["user"], row["host"]) {
			continue
		}
		if len(schemaScopeSet) > 0 {
			if _, found := schemaScopeSet[row["db_name"]]; !found {
				continue
			}
		}
		bundle := ensureBundle(bundles, row["user"], row["host"], matchMode)
		bundle.Hosts.Add(row["host"])
		privileges := bundle.DBPrivileges[row["db_name"]]
		if privileges == nil {
			privileges = StringSet{}
			bundle.DBPrivileges[row["db_name"]] = privileges
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
		if !hasIdentity(identities, row["user"], row["host"]) {
			continue
		}
		if len(schemaScopeSet) > 0 {
			if _, found := schemaScopeSet[row["db_name"]]; !found {
				continue
			}
		}
		bundle := ensureBundle(bundles, row["user"], row["host"], matchMode)
		bundle.Hosts.Add(row["host"])
		scope := TableScope{Schema: row["db_name"], Table: row["table_name"]}
		privileges := bundle.TablePrivileges[scope]
		if privileges == nil {
			privileges = StringSet{}
			bundle.TablePrivileges[scope] = privileges
		}
		privileges.Update(parsePrivilegeSet(row["table_priv"]))
	}

	for _, item := range users {
		ensureBundle(bundles, item[0], item[1], matchMode)
	}
	return bundles, nil
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

func parsePrivilegeSet(value string) []string {
	if value == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			set[strings.ToUpper(strings.ReplaceAll(trimmed, "_", " "))] = struct{}{}
		}
	}
	privileges := make([]string, 0, len(set))
	for privilege := range set {
		privileges = append(privileges, privilege)
	}
	sort.Strings(privileges)
	return privileges
}

func ensureBundle(bundles map[string]*PrivilegeBundle, user, host, matchMode string) *PrivilegeBundle {
	key := user + "@" + host
	identity := PrivilegeIdentity{User: user, Host: &host}
	if matchMode == "user" {
		key = user
		identity.Host = nil
	}
	bundle := bundles[key]
	if bundle == nil {
		bundle = newPrivilegeBundle(identity)
		bundles[key] = bundle
	}
	return bundle
}

func remapPrivilegeBundles(bundles map[string]*PrivilegeBundle, schemaNameMap map[string]string) map[string]*PrivilegeBundle {
	remapped := map[string]*PrivilegeBundle{}
	for key, bundle := range bundles {
		cloned := newPrivilegeBundle(bundle.Identity)
		cloned.GlobalPrivileges.Update(bundle.GlobalPrivileges.Sorted())
		cloned.Hosts.Update(bundle.Hosts.Sorted())
		for dbName, privileges := range bundle.DBPrivileges {
			normalizedName := schemaNameMap[dbName]
			if normalizedName == "" {
				normalizedName = dbName
			}
			target := cloned.DBPrivileges[normalizedName]
			if target == nil {
				target = StringSet{}
				cloned.DBPrivileges[normalizedName] = target
			}
			target.Update(privileges.Sorted())
		}
		for scope, privileges := range bundle.TablePrivileges {
			normalizedName := schemaNameMap[scope.Schema]
			if normalizedName == "" {
				normalizedName = scope.Schema
			}
			targetScope := TableScope{Schema: normalizedName, Table: scope.Table}
			target := cloned.TablePrivileges[targetScope]
			if target == nil {
				target = StringSet{}
				cloned.TablePrivileges[targetScope] = target
			}
			target.Update(privileges.Sorted())
		}
		remapped[key] = cloned
	}
	return remapped
}

func hasIdentity(identities map[string]struct{}, user, host string) bool {
	_, found := identities[user+"@"+host]
	return found
}
