package mysqlrenamedb

import (
	"flag"
	"os"
	"strings"
)

func Run(argv []string) (int, error) {
	options, err := parseArgs(argv)
	if err != nil {
		return 0, err
	}

	client, err := NewMySQLClient(options.Target, options.ConnectTimeout)
	if err != nil {
		return 0, err
	}
	defer client.Close()

	report := RunReport{
		DryRun: options.DryRun,
	}

	// Run pre-checks unless skipped
	if !options.SkipPreCheck {
		preCheck, err := runPreChecks(client, options.OldDBName, options.NewDBName)
		if err != nil {
			return 0, err
		}
		report.PreCheck = &preCheck

		// If critical checks failed, exit
		if !preCheck.Passed {
			output := renderReport(report, options.OutputFormat)
			if err := writeOutput(output, options.OutputPath); err != nil {
				return 0, err
			}
			return 1, nil
		}
	}

	// If dry-run, output and exit
	if options.DryRun {
		output := renderReport(report, options.OutputFormat)
		if err := writeOutput(output, options.OutputPath); err != nil {
			return 0, err
		}
		return 0, nil
	}

	// Execute rename
	result, err := renameDatabase(client, options.OldDBName, options.NewDBName)
	report.RenameResult = &result

	output := renderReport(report, options.OutputFormat)
	if err := writeOutput(output, options.OutputPath); err != nil {
		return 0, err
	}

	if !result.Success {
		return 2, nil
	}

	return 0, nil
}

func parseArgs(argv []string) (Options, error) {
	var host string
	var port int
	var user string
	var password string
	var socket string
	var configPath string
	var defaultsFile string
	var oldDBName string
	var newDBName string
	var skipPreCheck bool
	var dryRun bool
	var connectTimeout int
	var outputFormat string
	var outputPath string

	fs := flag.NewFlagSet("mysql-rename-db", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&host, "host", "", "MySQL host IP (required)")
	fs.IntVar(&port, "port", 0, "MySQL port (default: 3306)")
	fs.StringVar(&user, "user", "", "MySQL username")
	fs.StringVar(&password, "password", "", "MySQL password")
	fs.StringVar(&socket, "socket", "", "MySQL unix socket path")
	fs.StringVar(&configPath, "config", "", "JSON config file")
	fs.StringVar(&defaultsFile, "defaults-file", "", "MySQL defaults file (e.g., /etc/my.cnf)")
	fs.StringVar(&oldDBName, "old-dbname", "", "Source database name (required)")
	fs.StringVar(&newDBName, "new-dbname", "", "Target database name (required)")
	fs.BoolVar(&skipPreCheck, "skip-precheck", false, "Skip all pre-checks")
	fs.BoolVar(&dryRun, "dry-run", false, "Only run checks, don't rename")
	fs.IntVar(&connectTimeout, "connect-timeout", 5, "Connection timeout in seconds")
	fs.StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	fs.StringVar(&outputPath, "output", "", "Write output to file instead of stdout")

	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	// Validate required parameters
	if strings.TrimSpace(host) == "" && strings.TrimSpace(socket) == "" {
		return Options{}, newUsageError("--host or --socket is required")
	}
	if strings.TrimSpace(oldDBName) == "" {
		return Options{}, newUsageError("--old-dbname is required")
	}
	if strings.TrimSpace(newDBName) == "" {
		return Options{}, newUsageError("--new-dbname is required")
	}
	if oldDBName == newDBName {
		return Options{}, newUsageError("--old-dbname and --new-dbname cannot be the same")
	}
	if outputFormat != "text" && outputFormat != "json" {
		return Options{}, newUsageError("invalid --output-format, expected text or json")
	}

	// Load configs
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

	// Merge credentials
	cliCreds := DefaultsFileConfig{
		User:     user,
		Password: password,
		Host:     host,
		Port:     port,
		Socket:   socket,
	}
	mergedCreds := mergeCredentials(cliCreds, defaultsConfig, fileConfig)

	// Determine final connection config
	finalPort := mergedCreds.Port
	if finalPort == 0 {
		finalPort = fileConfig.DefaultPort
	}
	if finalPort == 0 {
		finalPort = 3306
	}

	finalHost := mergedCreds.Host
	if finalHost == "" {
		finalHost = host
	}

	if strings.TrimSpace(finalHost) == "" && strings.TrimSpace(mergedCreds.Socket) == "" {
		return Options{}, newUsageError("MySQL host or socket must be specified")
	}
	if strings.TrimSpace(mergedCreds.User) == "" {
		return Options{}, newUsageError("MySQL user must be specified")
	}

	target := ConnectionConfig{
		Host:     finalHost,
		Port:     finalPort,
		User:     mergedCreds.User,
		Password: mergedCreds.Password,
		Socket:   mergedCreds.Socket,
	}

	return Options{
		Target:         target,
		ConfigPath:     configPath,
		DefaultsFile:   defaultsFile,
		OldDBName:      oldDBName,
		NewDBName:      newDBName,
		SkipPreCheck:   skipPreCheck,
		DryRun:         dryRun,
		ConnectTimeout: connectTimeout,
		OutputFormat:   outputFormat,
		OutputPath:     outputPath,
	}, nil
}

func writeOutput(output string, path string) error {
	if strings.TrimSpace(path) == "" {
		_, err := os.Stdout.WriteString(output + "\n")
		return err
	}
	return os.WriteFile(path, []byte(output+"\n"), 0o644)
}
