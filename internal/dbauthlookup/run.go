package dbauthlookup

import (
	"flag"
	"os"
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
	output, err := renderReport(report, options.OutputFormat)
	if err != nil {
		return 0, err
	}
	_, _ = os.Stdout.WriteString(output + "\n")
	if report.Summary.AuthorizationCount == 0 {
		return 1, nil
	}
	return 0, nil
}

func parseArgs(argv []string) (Options, error) {
	var options Options
	fs := flag.NewFlagSet("db-auth-lookup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&options.BusinessName, "business-name", "", "Business name from 数据库集群映射表")
	fs.StringVar(&options.BusinessClusterFile, "business-cluster-file", "", "Path to 数据库集群映射表.xlsx")
	fs.StringVar(&options.DBClusterFile, "db-cluster-file", "", "Path to 数据库和集群映射表.xlsx")
	fs.StringVar(&options.AccessRelationFile, "access-relation-file", "", "Path to 访问关系表.xlsx")
	fs.StringVar(&options.AppIPFile, "app-ip-file", "", "Path to 应用和ip映射表.xlsx")
	fs.StringVar(&options.OutputFormat, "output-format", "text", "Output format: text or json")
	fs.BoolVar(&options.WithDiagnostics, "with-diagnostics", false, "Render unmatched and parse diagnostics")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	if options.BusinessName == "" {
		return Options{}, newUsageError("--business-name is required")
	}
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
	case "text", "json":
	default:
		return Options{}, newUsageError("invalid --output-format, expected text or json")
	}
	return options, nil
}
