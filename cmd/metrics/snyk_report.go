package metrics

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/danlafeir/em/internal/charts"
	snykpkg "github.com/danlafeir/em/pkg/snyk"
)

var snykReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a Snyk security metrics report as a single HTML page",
	Long: `Generate an HTML report of vulnerability counts and weekly trend.

Required:
  em metrics snyk config`,
	RunE: runSnykReport,
}

func init() {
	SnykCmd.AddCommand(snykReportCmd)
}

func runSnykReport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	from, to, err := getSnykDateRange()
	if err != nil {
		return err
	}

	var issues, resolved []snykpkg.Issue
	var openCounts snykpkg.OpenCounts

	if useSavedDataFlag {
		issues, resolved, openCounts, err = fetchOrLoadSnykData(ctx, nil, from, to)
		if err != nil {
			return fmt.Errorf("failed to load saved Snyk data: %w", err)
		}
	} else {
		client, err := getSnykClient()
		if err != nil {
			return err
		}
		fmt.Println("Testing Snyk connection...")
		if err := client.TestConnection(ctx); err != nil {
			return fmt.Errorf("failed to connect to Snyk: %w", err)
		}

		filter := snykpkg.IssueFilter{ScanItemType: getConfigString("snyk.scan_item_type")}

		fmt.Printf("Fetching open issues (%s to %s)...\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
		issues, err = client.ListIssues(ctx, from, to, filter)
		if err != nil {
			return fmt.Errorf("failed to fetch open issues: %w", err)
		}
		fmt.Printf("  %d issues found\n", len(issues))

		fmt.Println("Fetching resolved issues...")
		resolved, err = client.ListResolvedIssues(ctx, from, to, filter)
		if err != nil {
			return fmt.Errorf("failed to fetch resolved issues: %w", err)
		}
		fmt.Printf("  %d issues found\n", len(resolved))

		fmt.Println("Counting current open vulnerabilities...")
		openCounts, err = client.CountOpenIssues(ctx, filter)
		if err != nil {
			return fmt.Errorf("failed to count open issues: %w", err)
		}
		fmt.Printf("  Critical: %d, High: %d, Medium: %d, Low: %d\n",
			openCounts.Critical, openCounts.High, openCounts.Medium, openCounts.Low)

		if saveRawDataFlag {
			_ = saveSnykIssueList(issues, savedSnykIssuesPath())
			_ = saveSnykIssueList(resolved, savedSnykResolvedPath())
			_ = saveSnykOpenCounts(openCounts)
		}
	}

	summary := charts.SnykSummary{
		Critical:            openCounts.Critical,
		High:                openCounts.High,
		Medium:              openCounts.Medium,
		Low:                 openCounts.Low,
		Fixable:             openCounts.Fixable,
		Unfixable:           openCounts.Unfixable,
		IgnoredFixable:      openCounts.IgnoredFixable,
		IgnoredUnfixable:    openCounts.IgnoredUnfixable,
		FixableCritical:     openCounts.FixableCritical,
		FixableHigh:         openCounts.FixableHigh,
		FixableMedium:       openCounts.FixableMedium,
		FixableLow:          openCounts.FixableLow,
		ExploitableCritical: openCounts.ExploitableCritical,
		ExploitableHigh:     openCounts.ExploitableHigh,
		ExploitableMedium:   openCounts.ExploitableMedium,
		ExploitableLow:      openCounts.ExploitableLow,
		ExploitableFixable:          openCounts.ExploitableFixable,
		ExploitableUnfixable:        openCounts.ExploitableUnfixable,
		ExploitableIgnoredFixable:   openCounts.ExploitableIgnoredFixable,
		ExploitableIgnoredUnfixable: openCounts.ExploitableIgnoredUnfixable,
		ExploitableTotal:    openCounts.ExploitableCritical + openCounts.ExploitableHigh + openCounts.ExploitableMedium + openCounts.ExploitableLow,
	}

	weeks := bucketByWeek(issues, resolved, openCounts.Total, openCounts.Fixable, openCounts.IgnoredFixable, openCounts.IgnoredUnfixable, from, to)

	outputPath := getSnykOutputPath("snyk-report", "html")

	orgName := getConfigString("snyk.org_name")
	title := "Snyk Security Report"
	if team := getSelectedTeam(); team != "" {
		title = team + " — Snyk Security Report"
	} else if orgName != "" {
		title = orgName + " — Snyk Security Report"
	}

	fmt.Println("Generating report...")
	if err := charts.SnykSectionReport(summary, weeks, title, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("Report generated: %s\n", outputPath)
	openBrowser(outputPath)
	return nil
}
