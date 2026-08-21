package dbauthlookup

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
	dataset, err := loadDataset(options)
	if err != nil {
		return 0, err
	}
	report := buildReport(dataset, options)
	if options.OutputFormat == "xlsx" {
		if options.OutputPath == "" {
			return 0, newUsageError("--output is required when --output-format xlsx")
		}
		if err := writeXLSXReport(report, options.OutputPath); err != nil {
			return 0, err
		}
		writeConsoleOutput(report, options)
		if report.Summary.AuthorizationCount == 0 {
			return 1, nil
		}
		return 0, nil
	}
	if options.OutputFormat == "csv" && options.OutputPath == "" {
		return 0, newUsageError("--output is required when --output-format csv")
	}
	output, err := renderReport(report, options.OutputFormat)
	if err != nil {
		return 0, err
	}
	if options.OutputPath == "" {
		_, _ = os.Stdout.WriteString(output + "\n")
	} else {
		if err := os.WriteFile(options.OutputPath, []byte(output+"\n"), 0o644); err != nil {
			return 0, err
		}
		writeConsoleOutput(report, options)
	}
	if report.Summary.AuthorizationCount == 0 {
		return 1, nil
	}
	return 0, nil
}

func writeConsoleOutput(report Report, options Options) {
	consoleOutput := renderConsoleSummary(report)
	if options.WithDiagnostics {
		diagnostics := renderDiagnostics(report)
		if diagnostics != "" {
			consoleOutput += "\n" + diagnostics
		}
	}
	_, _ = os.Stdout.WriteString(consoleOutput)
}

func parseArgs(argv []string) (Options, error) {
	var options Options
	var businessNames multiValueFlag
	fs := flag.NewFlagSet("db-auth-lookup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Var(&businessNames, "business-name", "Business name from 数据库集群映射表; repeatable and supports comma separators; omit to match all businesses")
	fs.StringVar(&options.BusinessClusterFile, "business-cluster-file", "", "Path to 数据库集群映射表 (.xlsx, .xlsm, or .csv)")
	fs.StringVar(&options.DBClusterFile, "db-cluster-file", "", "Path to 数据库和集群映射表 (.xlsx, .xlsm, or .csv)")
	fs.StringVar(&options.AccessRelationFile, "access-relation-file", "", "Path to 访问关系表 (.xlsx, .xlsm, or .csv)")
	fs.StringVar(&options.AppIPFile, "app-ip-file", "", "Path to 应用和ip映射表 (.xlsx, .xlsm, or .csv)")
	fs.StringVar(&options.OutputFormat, "output-format", "text", "Output format: text, json, csv, or xlsx")
	fs.StringVar(&options.OutputPath, "output", "", "Output file path; stdout is used when omitted")
	fs.StringVar(&options.AggregateBy, "aggregate-by", "detail", "Aggregation level: detail, database, or cluster")
	fs.BoolVar(&options.WithDiagnostics, "with-diagnostics", false, "Render unmatched and parse diagnostics")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	options.BusinessNames = splitBusinessNames(businessNames)
	if options.BusinessClusterFile == "" {
		return Options{}, newUsageError("--business-cluster-file is required")
	}
	if options.DBClusterFile == "" {
		return Options{}, newUsageError("--db-cluster-file is required")
	}
	if options.AccessRelationFile == "" {
		return Options{}, newUsageError("--access-relation-file is required")
	}
	if options.AppIPFile == "" {
		return Options{}, newUsageError("--app-ip-file is required")
	}
	switch options.OutputFormat {
	case "text", "json", "csv", "xlsx":
	default:
		return Options{}, newUsageError("invalid --output-format, expected text, json, csv, or xlsx")
	}
	switch options.AggregateBy {
	case "detail", "database", "cluster":
	default:
		return Options{}, newUsageError("invalid --aggregate-by, expected detail, database, or cluster")
	}
	if (options.OutputFormat == "csv" || options.OutputFormat == "xlsx") && options.OutputPath == "" {
		return Options{}, newUsageError("--output is required when --output-format " + options.OutputFormat)
	}
	return options, nil
}

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

func splitBusinessNames(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			name := strings.TrimSpace(item)
			if name == "" {
				continue
			}
			key := cleanText(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, name)
		}
	}
	return result
}
