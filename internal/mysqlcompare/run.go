package mysqlcompare

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type multiValueFlag []string

func (m *multiValueFlag) String() string {
	if m == nil {
		return ""
	}
	return strings.Join(*m, ",")
}

func (m *multiValueFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func Run(argv []string) (int, error) {
	options, err := parseArgs(argv)
	if err != nil {
		return 0, err
	}
	if options.OutputFormat == "text" {
		fmt.Println("Source:", options.Source.DisplayName())
	}

	sourceClient, err := NewMySQLCLIClient(options.Source)
	if err != nil {
		return 0, err
	}
	targetClients := make([]DatabaseClient, 0, len(options.Targets))
	for _, target := range options.Targets {
		client, err := NewMySQLCLIClient(target)
		if err != nil {
			return 0, err
		}
		targetClients = append(targetClients, client)
	}

	sourceAvailableSchemas, err := resolveSchemas(sourceClient, nil, options.ExcludeSchemas)
	if err != nil {
		return 0, err
	}
	sourceSelectedSchemas, err := resolveSchemas(sourceClient, options.SourceSchemas, options.ExcludeSchemas)
	if err != nil {
		return 0, err
	}
	if len(options.SourceSchemas) == 0 {
		sourceSelectedSchemas = sourceAvailableSchemas
	}

	sourceSchemaCache := map[string]SchemaSnapshot{}
	sourceUsers, err := resolveUsers(sourceClient, options.Users, options.ExcludeUsers)
	if err != nil {
		return 0, err
	}

	comparisons := make([]TargetComparison, 0, len(options.Targets))
	for index, target := range options.Targets {
		targetClient := targetClients[index]
		comparison := TargetComparison{
			Target:            target.DisplayName(),
			TargetConfig:      target,
			IncludeStructure:  options.CompareStructure,
			IncludePrivileges: options.ComparePrivileges,
		}

		targetAvailableSchemas, err := resolveSchemas(targetClient, nil, options.ExcludeSchemas)
		if err != nil {
			comparison.Error = err.Error()
			comparisons = append(comparisons, comparison)
			fmt.Println(renderTargetReport(comparison, options.OutputFormat))
			continue
		}
		targetSelectedSchemas, err := resolveSchemas(targetClient, options.TargetSchemas, options.ExcludeSchemas)
		if err != nil {
			comparison.Error = err.Error()
			comparisons = append(comparisons, comparison)
			fmt.Println(renderTargetReport(comparison, options.OutputFormat))
			continue
		}
		if len(options.TargetSchemas) == 0 {
			targetSelectedSchemas = targetAvailableSchemas
		}
		schemaPairs, err := mapSchemaPairs(
			sourceAvailableSchemas,
			sourceSelectedSchemas,
			options.SourceSchemas,
			targetAvailableSchemas,
			targetSelectedSchemas,
			options.TargetSchemas,
		)
		if err != nil {
			comparison.Error = err.Error()
			comparisons = append(comparisons, comparison)
			fmt.Println(renderTargetReport(comparison, options.OutputFormat))
			continue
		}
		comparison.SchemaPairs = schemaPairs

		if options.CompareStructure {
			targetSchemaCache := map[string]SchemaSnapshot{}
			for _, pair := range schemaPairs {
				if _, found := sourceSchemaCache[pair.SourceSchema]; !found {
					snapshot, err := collectSchemaSnapshot(sourceClient, pair.SourceSchema)
					if err != nil {
						comparison.Error = err.Error()
						break
					}
					sourceSchemaCache[pair.SourceSchema] = snapshot
				}
				if _, found := targetSchemaCache[pair.TargetSchema]; !found {
					snapshot, err := collectSchemaSnapshot(targetClient, pair.TargetSchema)
					if err != nil {
						comparison.Error = err.Error()
						break
					}
					targetSchemaCache[pair.TargetSchema] = snapshot
				}
				schemaDiff := diffSchema(sourceSchemaCache[pair.SourceSchema], targetSchemaCache[pair.TargetSchema])
				if schemaDiff.HasChanges() {
					comparison.SchemaDiffs = append(comparison.SchemaDiffs, schemaDiff)
				}
			}
		}

		if comparison.Error == "" && options.ComparePrivileges {
			targetUsers, err := resolveUsers(targetClient, options.Users, options.ExcludeUsers)
			if err != nil {
				comparison.Error = err.Error()
			} else {
				sourceScope := []string{}
				targetScope := []string{}
				targetToSource := map[string]string{}
				for _, pair := range schemaPairs {
					sourceScope = append(sourceScope, pair.SourceSchema)
					targetScope = append(targetScope, pair.TargetSchema)
					targetToSource[pair.TargetSchema] = pair.SourceSchema
				}
				sourcePrivileges, err := collectPrivileges(sourceClient, sourceUsers, options.UserMatchMode, uniqSorted(sourceScope))
				if err != nil {
					comparison.Error = err.Error()
				} else {
					targetPrivileges, err := collectPrivileges(targetClient, targetUsers, options.UserMatchMode, uniqSorted(targetScope))
					if err != nil {
						comparison.Error = err.Error()
					} else {
						comparison.PrivilegeDiff = diffPrivileges(sourcePrivileges, remapPrivilegeBundles(targetPrivileges, targetToSource))
					}
				}
			}
		}

		comparisons = append(comparisons, comparison)
		fmt.Println(renderTargetReport(comparison, options.OutputFormat))
	}

	summary := buildSummary(comparisons)
	exitCode := determineExitCode(summary)
	fmt.Println(renderSummaryReport(summary, options.OutputFormat, exitCode))
	return exitCode, nil
}

func parseArgs(argv []string) (Options, error) {
	var sourceDSN string
	var targetDSNs multiValueFlag
	var configPath string
	var defaultUser string
	var defaultPassword string
	var sourceSchemas multiValueFlag
	var targetSchemas multiValueFlag
	var excludeSchemas multiValueFlag
	var users multiValueFlag
	var excludeUsers multiValueFlag
	var userMatchMode string
	var checkMode string
	var outputFormat string

	fs := flag.NewFlagSet("mysqlcompare", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&sourceDSN, "source-dsn", "", "Source MySQL DSN")
	fs.Var(&targetDSNs, "target-dsn", "Target MySQL DSN, repeatable and supports , | newline separators")
	fs.StringVar(&configPath, "config", "", "JSON config file, supports default_user and default_password")
	fs.StringVar(&defaultUser, "default-user", "", "Default MySQL user")
	fs.StringVar(&defaultPassword, "default-password", "", "Default MySQL password")
	fs.Var(&sourceSchemas, "source-schemas", "Source schema selectors")
	fs.Var(&sourceSchemas, "source-databases", "Source database selectors")
	fs.Var(&targetSchemas, "target-schemas", "Target schema selectors")
	fs.Var(&targetSchemas, "target-databases", "Target database selectors")
	fs.Var(&excludeSchemas, "exclude-schemas", "Schema selectors to exclude")
	fs.Var(&excludeSchemas, "exclude-databases", "Database selectors to exclude")
	fs.Var(&users, "users", "User or user@host selectors")
	fs.Var(&excludeUsers, "exclude-users", "User or user@host selectors to exclude")
	fs.StringVar(&userMatchMode, "user-match-mode", "user_host", "Privilege match mode: user or user_host")
	fs.StringVar(&checkMode, "check", "all", "Check mode: all, structure, privileges")
	fs.StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	if sourceDSN == "" {
		return Options{}, newUsageError("missing required --source-dsn")
	}
	fileConfig, err := loadFileConfig(configPath)
	if err != nil {
		return Options{}, err
	}
	if defaultUser == "" {
		defaultUser = fileConfig.DefaultUser
	}
	if defaultPassword == "" {
		defaultPassword = fileConfig.DefaultPassword
	}
	source, err := parseConnectionDSN(sourceDSN, "", defaultUser, defaultPassword)
	if err != nil {
		return Options{}, err
	}
	targets, err := parseTargetDSNs(targetDSNs, defaultUser, defaultPassword)
	if err != nil {
		return Options{}, err
	}
	if userMatchMode != "user" && userMatchMode != "user_host" {
		return Options{}, newUsageError("invalid --user-match-mode, expected user or user_host")
	}
	compareStructure := false
	comparePrivileges := false
	switch checkMode {
	case "all":
		compareStructure = true
		comparePrivileges = true
	case "structure":
		compareStructure = true
	case "privileges":
		comparePrivileges = true
	default:
		return Options{}, newUsageError("invalid --check, expected all, structure, or privileges")
	}
	if outputFormat != "text" && outputFormat != "json" {
		return Options{}, newUsageError("invalid --output-format, expected text or json")
	}

	return Options{
		Source:            source,
		Targets:           targets,
		ConfigPath:        configPath,
		SourceSchemas:     splitMultiValue(sourceSchemas),
		TargetSchemas:     splitMultiValue(targetSchemas),
		ExcludeSchemas:    splitMultiValue(excludeSchemas),
		Users:             splitMultiValue(users),
		ExcludeUsers:      splitMultiValue(excludeUsers),
		UserMatchMode:     userMatchMode,
		CompareStructure:  compareStructure,
		ComparePrivileges: comparePrivileges,
		OutputFormat:      outputFormat,
	}, nil
}

func buildSummary(comparisons []TargetComparison) ComparisonSummary {
	totalTargets := len(comparisons)
	failedTargets := 0
	inconsistentTargets := 0
	failedTargetDetails := []TargetSummaryDetail{}
	inconsistentDetails := []TargetSummaryDetail{}
	for _, comparison := range comparisons {
		if !comparison.IsSuccessful() {
			failedTargets++
			failedTargetDetails = append(failedTargetDetails, buildTargetSummaryDetail(comparison))
		}
		if comparison.HasDifferences() {
			inconsistentTargets++
			inconsistentDetails = append(inconsistentDetails, buildTargetSummaryDetail(comparison))
		}
	}
	successfulTargets := totalTargets - failedTargets
	return ComparisonSummary{
		TotalTargets:        totalTargets,
		SuccessfulTargets:   successfulTargets,
		FailedTargets:       failedTargets,
		ConsistentTargets:   successfulTargets - inconsistentTargets,
		InconsistentTargets: inconsistentTargets,
		FailedTargetDetails: failedTargetDetails,
		InconsistentDetails: inconsistentDetails,
	}
}

func buildTargetSummaryDetail(comparison TargetComparison) TargetSummaryDetail {
	comparedSchemas := []string{}
	for _, pair := range comparison.SchemaPairs {
		comparedSchemas = append(comparedSchemas, pair.SourceSchema+"->"+pair.TargetSchema)
	}
	return TargetSummaryDetail{
		Target:          comparison.Target,
		Host:            comparison.TargetConfig.Host,
		Port:            comparison.TargetConfig.Port,
		Database:        comparison.TargetConfig.Database,
		ComparedSchemas: comparedSchemas,
		Error:           comparison.Error,
	}
}

func determineExitCode(summary ComparisonSummary) int {
	if summary.FailedTargets > 0 {
		return 2
	}
	if summary.InconsistentTargets > 0 {
		return 1
	}
	return 0
}
