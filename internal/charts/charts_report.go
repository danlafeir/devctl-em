package charts

import (
	"fmt"
	"html/template"
	"sort"

	"github.com/danlafeir/em/pkg/execreport"
	"github.com/danlafeir/em/pkg/metrics"
)

// ReportSummary holds the key metrics displayed in the summary bar.
type ReportSummary struct {
	AvgCycleTime    string
	AvgThroughput   string
	ActiveEpics     int
	Weeks           int
	CycleTimeTrend  string // "up", "down", "flat", or ""
	ThroughputTrend string
}

// ReportSummaryHTML returns a self-contained HTML fragment for the summary bar.
func ReportSummaryHTML(s ReportSummary) (template.HTML, error) {
	return renderHTML("fragment_summary.html.tmpl", s)
}

// CombinedReport renders a 2x2 HTML report with cycle time, throughput, longest CT, and forecast.
func CombinedReport(
	summary ReportSummary,
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	jiraBaseURL string,
	path string,
) error {
	return writeHTML(path, "report.html.tmpl", map[string]any{
		"SummaryHTML":    chartOrError(ReportSummaryHTML(summary)),
		"CycleTimeHTML":  chartOrError(CycleTimeScatterHTML(cycleTimeData, cycleTimePercentiles, "Cycle Time Distribution")),
		"ThroughputHTML": chartOrError(ThroughputLineHTML(throughputData, "Weekly Throughput")),
		"LongestCTHTML":  chartOrError(LongestCycleTimeTableHTML(longestCTRows, "Longest Cycle Times", jiraBaseURL)),
		"ForecastHTML":   chartOrError(ForecastTableHTML(forecastRows, "Epic Forecast", jiraBaseURL)),
	})
}

// CombinedTeamReport renders an HTML report combining GitHub deployment frequency,
// JIRA metrics, and Snyk vulnerability sections.
func CombinedTeamReport(
	title string,
	summary ReportSummary,
	deploymentData metrics.ThroughputResult,
	deploymentFailures metrics.ThroughputResult,
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	jiraBaseURL string,
	snykSummary SnykSummary,
	snykWeeks []SnykIssueWeek,
	path string,
) error {
	var dfHTML, dfSummaryHTML template.HTML
	if len(deploymentData.Periods) > 0 {
		dfHTML = chartOrError(DeploymentFrequencyLineHTML(deploymentData, deploymentFailures, "Deployment Frequency"))
		dfSummaryHTML = chartOrError(DeploymentSummaryHTML(deploymentData, deploymentFailures, summary.Weeks))
	}

	var snykSummaryHTML, snykChartHTML template.HTML
	if len(snykWeeks) > 0 {
		snykSummaryHTML = chartOrError(SnykSummaryHTML(snykSummary))
		snykChartHTML = chartOrError(SnykIssuesLineHTML(snykWeeks, "Open Snyk Issues over time"))
	}

	// Build executive healthcheck from available data.
	avgDeployFreq := "—"
	lastWeekDeploys := 0
	if deploymentData.AvgCount > 0 {
		avgDeployFreq = fmt.Sprintf("%.1f", deploymentData.AvgCount)
	}
	if n := len(deploymentData.Periods); n > 0 {
		lastWeekDeploys = deploymentData.Periods[n-1].Count
	}
	hc := execreport.ExecHealthcheck{
		AvgCycleTime:         summary.AvgCycleTime,
		AvgThroughput:        summary.AvgThroughput,
		ActiveEpics:          summary.ActiveEpics,
		HasJIRAData:          len(cycleTimeData) > 0 || throughputData.AvgCount > 0,
		AvgDeployFreq:        avgDeployFreq,
		LastWeekDeploys:      lastWeekDeploys,
		HasDeployData:        len(deploymentData.Periods) > 0,
		Critical:            snykSummary.Critical,
		FixableCritical:     snykSummary.FixableCritical,
		ExploitableCritical: snykSummary.ExploitableCritical,
		High:                snykSummary.High,
		FixableHigh:         snykSummary.FixableHigh,
		ExploitableHigh:     snykSummary.ExploitableHigh,
		HasSnykData:          len(snykWeeks) > 0,
		Weeks:                summary.Weeks,
		CycleTimeTrend:       cycleTimeTrend(cycleTimeData),
		ThroughputTrend:      periodsTrend(throughputData.Periods),
		DeployFreqTrend:      periodsTrend(deploymentData.Periods),
	}

	return writeHTML(path, "team_report.html.tmpl", map[string]any{
		"Title":               title,
		"ExecHealthcheckHTML": chartOrError(execreport.ExecHealthcheckHTML(hc)),
		"SummaryHTML":         chartOrError(ReportSummaryHTML(summary)),
		"DeploymentSummaryHTML": dfSummaryHTML,
		"DeploymentHTML":        dfHTML,
		"CycleTimeHTML":       chartOrError(CycleTimeScatterHTML(cycleTimeData, cycleTimePercentiles, "Cycle Time Distribution")),
		"ThroughputHTML":      chartOrError(ThroughputLineHTML(throughputData, "Weekly Throughput")),
		"LongestCTHTML":       chartOrError(LongestCycleTimeTableHTML(longestCTRows, "Longest Cycle Times", jiraBaseURL)),
		"ForecastHTML":        chartOrError(ForecastTableHTML(forecastRows, "Epic Forecast", jiraBaseURL)),
		"SnykSummaryHTML":     snykSummaryHTML,
		"SnykChartHTML":       snykChartHTML,
		"DatadogHTML":         "",
	})
}

// halfTrend returns "up", "down", or "flat" by comparing the mean of the second
// half of values against the first half. Returns "" when there is insufficient data.
// A change of more than 10% in either direction is considered a trend.
func halfTrend(values []float64) string {
	if len(values) < 2 {
		return ""
	}
	mid := len(values) / 2
	first, second := values[:mid], values[mid:]
	var a, b float64
	for _, v := range first {
		a += v
	}
	for _, v := range second {
		b += v
	}
	a /= float64(len(first))
	b /= float64(len(second))
	if a == 0 {
		return ""
	}
	change := (b - a) / a
	if change > 0.10 {
		return "up"
	}
	if change < -0.10 {
		return "down"
	}
	return "flat"
}

// PeriodsTrendFrom returns the trend direction for a slice of throughput periods.
func PeriodsTrendFrom(periods []metrics.ThroughputPeriod) string {
	vals := make([]float64, len(periods))
	for i, p := range periods {
		vals[i] = float64(p.Count)
	}
	return halfTrend(vals)
}

// periodsTrend is the unexported alias used within this package.
func periodsTrend(periods []metrics.ThroughputPeriod) string { return PeriodsTrendFrom(periods) }

// cycleTimeTrend is the unexported alias used within this package.
func cycleTimeTrend(results []metrics.CycleTimeResult) string { return CycleTimeTrendFrom(results) }

// CycleTimeTrendFrom returns the trend direction from cycle time results sorted by end date.
func CycleTimeTrendFrom(results []metrics.CycleTimeResult) string {
	if len(results) < 2 {
		return ""
	}
	sorted := make([]metrics.CycleTimeResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].EndDate.Before(sorted[j].EndDate)
	})
	vals := make([]float64, len(sorted))
	for i, r := range sorted {
		vals[i] = r.CycleTime.Hours()
	}
	return halfTrend(vals)
}
