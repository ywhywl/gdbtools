package mysqlpricheck

import (
	"encoding/json"
	"strings"
)

func renderReport(report RunReport, outputFormat string, detailLimit int) string {
	if outputFormat == "json" {
		data, _ := json.MarshalIndent(report, "", "  ")
		return string(data)
	}
	lines := []string{}
	for _, instance := range report.Reports {
		lines = append(lines, renderInstanceReportText(instance, detailLimit)...)
	}
	lines = append(lines, "Exit code: "+itoa(report.ExitCode))
	return strings.Join(lines, "\n")
}

func renderInstanceReportText(report InstanceReport, detailLimit int) []string {
	lines := []string{"Instance: " + report.Instance}
	if report.Error != "" {
		return append(lines, "  Status: failed", "  Error: "+report.Error)
	}
	lines = append(lines,
		"  Status: success",
		"  Summary:",
		"    checked_users="+itoa(report.Summary.CheckedUsers),
		"    checked_identities="+itoa(report.Summary.CheckedIdentities),
		"    inconsistent_host_privilege_users="+itoa(report.Summary.InconsistentHostPrivilegeUsers),
		"    multi_schema_identities="+itoa(report.Summary.MultiSchemaIdentities),
		"    db_level_privilege_identities="+itoa(report.Summary.DBLevelPrivilegeIdentities),
		"    table_level_privilege_identities="+itoa(report.Summary.TableLevelPrivilegeIdentities),
	)
	if len(report.Findings) == 0 {
		return append(lines, "  Findings: none")
	}
	lines = append(lines, "  Findings:")
	limit := detailLimit
	if limit <= 0 {
		limit = len(report.Findings)
	}
	for _, finding := range report.Findings[:min(limit, len(report.Findings))] {
		lines = append(lines, renderFindingText(finding)...)
	}
	if len(report.Findings) > limit {
		lines = append(lines, "    omitted_findings="+itoa(len(report.Findings)-limit))
	}
	return lines
}

func renderFindingText(finding Finding) []string {
	lines := []string{"    [" + strings.ToUpper(finding.Severity) + "] " + finding.Rule}
	if finding.User != "" {
		lines = append(lines, "      user="+finding.User)
	}
	if finding.Identity != "" {
		lines = append(lines, "      identity="+finding.Identity)
	}
	lines = append(lines, "      summary="+finding.Summary)
	detail, _ := json.Marshal(finding.Details)
	lines = append(lines, "      detail="+string(detail))
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
