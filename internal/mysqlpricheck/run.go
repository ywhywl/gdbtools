package mysqlpricheck

import (
	"flag"
	"os"
	"strings"
	"time"
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
	reports := make([]InstanceReport, 0, len(options.Targets))
	hadError := false
	for _, target := range options.Targets {
		client, err := NewMySQLClient(target, options.ConnectTimeout)
		if err != nil {
			hadError = true
			reports = append(reports, InstanceReport{Instance: target.DisplayName(), Error: err.Error()})
			continue
		}
		report, err := auditInstance(client, target.DisplayName(), options)
		_ = client.Close()
		if err != nil {
			hadError = true
			reports = append(reports, InstanceReport{Instance: target.DisplayName(), Error: err.Error()})
			continue
		}
		reports = append(reports, report)
	}
	exitCode := determineExitCode(reports, options.FailOn)
	if hadError {
		exitCode = 3
	}
	output := renderReport(RunReport{
		Reports:   reports,
		ExitCode:  exitCode,
		FailOn:    options.FailOn,
		Generated: time.Now().Format(time.RFC3339),
	}, options.OutputFormat, options.DetailLimit)
	if strings.TrimSpace(options.OutputPath) == "" {
		_, _ = os.Stdout.WriteString(output + "\n")
	} else {
		if err := os.WriteFile(options.OutputPath, []byte(output+"\n"), 0o644); err != nil {
			return 0, err
		}
	}
	return exitCode, nil
}

func parseArgs(argv []string) (Options, error) {
	var targets multiValueFlag
	var users multiValueFlag
	var excludeUsers multiValueFlag
	var excludeSchemas multiValueFlag
	var configPath string
	var defaultsFile string
	var user string
	var password string
	var socket string
	var port int
	var connectTimeout int
	var includeAnonymous bool
	var checkMode string
	var outputFormat string
	var outputPath string
	var detailLimit int
	var failOn string

	fs := flag.NewFlagSet("mysqlpricheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Var(&targets, "target", "Target MySQL address, repeatable and supports , | newline separators")
	fs.StringVar(&configPath, "config", "", "JSON config file")
	fs.StringVar(&defaultsFile, "defaults-file", "", "MySQL defaults file, such as /etc/my.cnf")
	fs.StringVar(&user, "user", "", "MySQL username")
	fs.StringVar(&password, "password", "", "MySQL password")
	fs.StringVar(&socket, "socket", "", "MySQL unix socket")
	fs.IntVar(&port, "port", 0, "Default MySQL port")
	fs.IntVar(&connectTimeout, "connect-timeout", 5, "Connect timeout in seconds")
	fs.Var(&users, "users", "User or user@host selectors")
	fs.Var(&excludeUsers, "exclude-users", "User or user@host selectors to exclude")
	fs.Var(&excludeSchemas, "exclude-schemas", "Schema selectors to exclude")
	fs.BoolVar(&includeAnonymous, "include-anonymous", false, "Include anonymous users")
	fs.StringVar(&checkMode, "check", "all", "Check mode: all, host_consistency, multi_schema, db_level, table_level")
	fs.StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	fs.StringVar(&outputPath, "output", "", "Output file path")
	fs.IntVar(&detailLimit, "detail-limit", 20, "Maximum findings to render per instance")
	fs.StringVar(&failOn, "fail-on", "medium", "Fail threshold: high, medium, low, none")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	if outputFormat != "text" && outputFormat != "json" {
		return Options{}, newUsageError("invalid --output-format, expected text or json")
	}
	switch checkMode {
	case "all", "host_consistency", "multi_schema", "db_level", "table_level":
	default:
		return Options{}, newUsageError("invalid --check")
	}
	switch failOn {
	case "high", "medium", "low", "none":
	default:
		return Options{}, newUsageError("invalid --fail-on")
	}

	fileConfig, err := loadFileConfig(configPath)
	if err != nil {
		return Options{}, err
	}
	defaultsConfig := DefaultsFileConfig{}
	if defaultsFile != "" {
		defaultsConfig, err = loadMyCnf(defaultsFile)
		if err != nil {
			return Options{}, err
		}
	} else {
		defaultsConfig, _, err = loadAutoDefaultsFile()
		if err != nil {
			return Options{}, err
		}
	}
	cliCreds := DefaultsFileConfig{User: user, Password: password, Port: port, Socket: socket}
	mergedCreds := mergeCredentials(cliCreds, defaultsConfig, fileConfig)
	defaultPort := mergedCreds.Port
	if defaultPort == 0 {
		defaultPort = fileConfig.DefaultPort
	}
	if defaultPort == 0 {
		defaultPort = 3306
	}
	targetConfigs, err := parseTargetList(targets, mergedCreds, defaultPort)
	if err != nil {
		return Options{}, err
	}
	excludedSchemas := append([]string{}, defaultExcludedSchemas...)
	if len(fileConfig.ExcludeSchemas) > 0 {
		excludedSchemas = append(excludedSchemas, fileConfig.ExcludeSchemas...)
	}
	excludedSchemas = uniqSorted(append(excludedSchemas, splitMultiValue(excludeSchemas)...))
	excludedUsers := splitMultiValue(excludeUsers)
	if len(fileConfig.ExcludeUsers) > 0 {
		excludedUsers = append(excludedUsers, fileConfig.ExcludeUsers...)
	}
	return Options{
		Targets:          targetConfigs,
		ConfigPath:       configPath,
		DefaultsFile:     defaultsFile,
		User:             mergedCreds.User,
		Password:         mergedCreds.Password,
		Port:             defaultPort,
		Socket:           mergedCreds.Socket,
		ConnectTimeout:   time.Duration(connectTimeout) * time.Second,
		Users:            users,
		ExcludeUsers:     excludedUsers,
		ExcludeSchemas:   excludedSchemas,
		IncludeAnonymous: includeAnonymous,
		CheckMode:        checkMode,
		OutputFormat:     outputFormat,
		OutputPath:       outputPath,
		DetailLimit:      detailLimit,
		FailOn:           failOn,
	}, nil
}

func determineExitCode(reports []InstanceReport, failOn string) int {
	if failOn == "none" {
		return 0
	}
	hasHigh := false
	hasMedium := false
	hasLow := false
	for _, report := range reports {
		for _, finding := range report.Findings {
			switch finding.Severity {
			case "high":
				hasHigh = true
			case "medium":
				hasMedium = true
			default:
				hasLow = true
			}
		}
	}
	if hasHigh {
		return 2
	}
	switch failOn {
	case "high":
		return 0
	case "medium":
		if hasMedium {
			return 1
		}
	case "low":
		if hasMedium || hasLow {
			return 1
		}
	}
	return 0
}
