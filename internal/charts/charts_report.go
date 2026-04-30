package charts

import (
	"fmt"
	"html/template"
	"sort"
	"time"

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

// trendDirection classifies a fractional change into "up", "down", or "flat".
// A move of more than 10% in either direction is considered a trend.
func trendDirection(change float64) string {
	if change > 0.10 {
		return "up"
	}
	if change < -0.10 {
		return "down"
	}
	return "flat"
}

// lastPeriodTrend compares the most recent period's value against the mean of
// all prior periods. Returns "" when there is insufficient data.
func lastPeriodTrend(values []float64) string {
	if len(values) < 2 {
		return ""
	}
	last := values[len(values)-1]
	prior := values[:len(values)-1]
	var avg float64
	for _, v := range prior {
		avg += v
	}
	avg /= float64(len(prior))
	if avg == 0 {
		return ""
	}
	return trendDirection((last - avg) / avg)
}

// PeriodsTrendFrom returns the trend direction for a slice of throughput periods
// by comparing the most recent period against the mean of all prior periods.
func PeriodsTrendFrom(periods []metrics.ThroughputPeriod) string {
	vals := make([]float64, len(periods))
	for i, p := range periods {
		vals[i] = float64(p.Count)
	}
	return lastPeriodTrend(vals)
}

// periodsTrend is the unexported alias used within this package.
func periodsTrend(periods []metrics.ThroughputPeriod) string { return PeriodsTrendFrom(periods) }

// cycleTimeTrend is the unexported alias used within this package.
func cycleTimeTrend(results []metrics.CycleTimeResult) string { return CycleTimeTrendFrom(results) }

// CycleTimeTrendFrom compares the mean cycle time of issues completed in the
// most recent 7 days against the mean of all prior issues.
func CycleTimeTrendFrom(results []metrics.CycleTimeResult) string {
	if len(results) < 2 {
		return ""
	}
	sorted := make([]metrics.CycleTimeResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].EndDate.Before(sorted[j].EndDate)
	})

	cutoff := sorted[len(sorted)-1].EndDate.Add(-7 * 24 * time.Hour)

	var recentSum, priorSum float64
	var recentN, priorN int
	for _, r := range sorted {
		h := r.CycleTime.Hours()
		if r.EndDate.After(cutoff) {
			recentSum += h
			recentN++
		} else {
			priorSum += h
			priorN++
		}
	}
	if recentN == 0 || priorN == 0 {
		return ""
	}
	priorMean := priorSum / float64(priorN)
	if priorMean == 0 {
		return ""
	}
	recentMean := recentSum / float64(recentN)
	return trendDirection((recentMean - priorMean) / priorMean)
}
