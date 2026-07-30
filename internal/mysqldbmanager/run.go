package mysqldbmanager

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func Run(argv []string) (int, error) {
	if len(argv) == 0 {
		printUsage()
		return 0, newUsageError("command required: rename or drop")
	}

	command := argv[0]
	if command != "rename" && command != "drop" {
		printUsage()
		return 0, newUsageError(fmt.Sprintf("unknown command: %s", command))
	}

	options, err := parseArgs(command, argv[1:])
	if err != nil {
		return 0, err
	}

	client, err := NewMySQLClient(options.Target, options.ConnectTimeout)
	if err != nil {
		return 0, err
	}
	defer client.Close()

	report := RunReport{
		Command: string(options.Command),
		DryRun:  options.DryRun,
	}

	// Run pre-checks
	shouldRunChecks := true
	if options.SkipPreCheck {
		shouldRunChecks = false
	}

	if shouldRunChecks {
		preCheck, err := runPreChecks(client, options)
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

	// Execute operation based on command
	if options.Command == CommandRename {
		result, _ := renameDatabase(client, options.OldDBName, options.NewDBName)
		report.RenameResult = &result

		output := renderReport(report, options.OutputFormat)
		if err := writeOutput(output, options.OutputPath); err != nil {
			return 0, err
		}

		if !result.Success {
			return 2, nil
		}
	} else if options.Command == CommandDrop {
		result, _ := dropDatabase(client, options)
		report.DropResult = &result

		output := renderReport(report, options.OutputFormat)
		if err := writeOutput(output, options.OutputPath); err != nil {
			return 0, err
		}

		if !result.Success {
			return 2, nil
		}
	}

	return 0, nil
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: mysql-db-manager <command> [options]

Commands:
  rename    Rename a database using RENAME TABLE
  drop      Drop a database (name must end with 'bak')

Options:
  --host <ip>                MySQL host IP (required unless --socket)
  --port <port>              MySQL port (default: 3306)
  --user <user>              MySQL username
  --password <pass>          MySQL password
  --socket <path>            MySQL unix socket path
  --config <file>            JSON config file
  --defaults-file <file>     MySQL defaults file (e.g., /etc/my.cnf)
  --old-dbname <name>        Source database name (required)
  --new-dbname <name>        Target database name (rename only, default: old-dbname + "bak")
  --skip-precheck            Skip all pre-checks
  --skip-business-checks     Skip business checks (connections, modifications, locks, replication)
  --dry-run                  Show what would happen
  --output-format <fmt>      Output format: text or json (default: text)
  --output <file>            Write output to file instead of stdout
  --batch-size <n>           Drop tables in batches (drop mode only, 0=drop database directly, default: 1)
  --sleep-interval <ms>      Milliseconds to sleep between batches (drop mode only, default: 100)

Examples:
  # Rename database (dry-run first)
  mysql-db-manager rename --host 192.168.1.100 --user root --old-dbname app_db --dry-run
  mysql-db-manager rename --host 192.168.1.100 --user root --old-dbname app_db --new-dbname app_db_new

  # Rename with default new name (app_db -> app_dbbak)
  mysql-db-manager rename --host 192.168.1.100 --user root --old-dbname app_db

  # Drop database (checks connections/modifications by default)
  mysql-db-manager drop --host 192.168.1.100 --user root --old-dbname app_db_bak --dry-run
  mysql-db-manager drop --host 192.168.1.100 --user root --old-dbname app_db_bak

  # Drop database skipping business checks
  mysql-db-manager drop --host 192.168.1.100 --user root --old-dbname app_db_bak --skip-business-checks

  # Drop database gradually (batch mode to reduce impact on other databases)
  mysql-db-manager drop --host 192.168.1.100 --user root --old-dbname app_db_bak --batch-size 100 --sleep-interval 500
`)
}

func parseArgs(command string, argv []string) (Options, error) {
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
	var skipBusinessChecks bool
	var dryRun bool
	var connectTimeout int
	var outputFormat string
	var outputPath string
	var batchSize int
	var sleepInterval int

	fs := flag.NewFlagSet("mysql-db-manager "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&host, "host", "", "MySQL host IP")
	fs.IntVar(&port, "port", 0, "MySQL port (default: 3306)")
	fs.StringVar(&user, "user", "", "MySQL username")
	fs.StringVar(&password, "password", "", "MySQL password")
	fs.StringVar(&socket, "socket", "", "MySQL unix socket path")
	fs.StringVar(&configPath, "config", "", "JSON config file")
	fs.StringVar(&defaultsFile, "defaults-file", "", "MySQL defaults file (e.g., /etc/my.cnf)")
	fs.StringVar(&oldDBName, "old-dbname", "", "Source database name (required)")
	fs.StringVar(&newDBName, "new-dbname", "", "Target database name (rename only)")
	fs.BoolVar(&skipPreCheck, "skip-precheck", false, "Skip all pre-checks")
	fs.BoolVar(&skipBusinessChecks, "skip-business-checks", false, "Skip business checks")
	fs.BoolVar(&dryRun, "dry-run", false, "Show what would happen")
	fs.IntVar(&connectTimeout, "connect-timeout", 5, "Connection timeout in seconds")
	fs.StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	fs.StringVar(&outputPath, "output", "", "Write output to file instead of stdout")
	fs.IntVar(&batchSize, "batch-size", 1, "Drop tables in batches (0=drop database directly, drop mode only)")
	fs.IntVar(&sleepInterval, "sleep-interval", 100, "Milliseconds to sleep between batches (drop mode only)")

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

	// Command-specific validation
	if command == "rename" {
		if strings.TrimSpace(newDBName) == "" {
			newDBName = oldDBName + "bak"
		}
		if oldDBName == newDBName {
			return Options{}, newUsageError("--old-dbname and --new-dbname cannot be the same")
		}
	} else if command == "drop" {
		if strings.TrimSpace(newDBName) != "" {
			return Options{}, newUsageError("--new-dbname is not applicable in drop mode")
		}
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

	cmd := CommandRename
	if command == "drop" {
		cmd = CommandDrop
	}

	return Options{
		Command:            cmd,
		Target:             target,
		ConfigPath:         configPath,
		DefaultsFile:       defaultsFile,
		OldDBName:          oldDBName,
		NewDBName:          newDBName,
		SkipPreCheck:       skipPreCheck,
		SkipBusinessChecks: skipBusinessChecks,
		DryRun:             dryRun,
		ConnectTimeout:     connectTimeout,
		OutputFormat:       outputFormat,
		OutputPath:         outputPath,
		BatchSize:          batchSize,
		SleepInterval:      sleepInterval,
	}, nil
}

func writeOutput(output string, path string) error {
	if strings.TrimSpace(path) == "" {
		_, err := os.Stdout.WriteString(output + "\n")
		return err
	}
	return os.WriteFile(path, []byte(output+"\n"), 0o644)
}
