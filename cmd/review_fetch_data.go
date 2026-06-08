package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/danlafeir/cli-go/pkg/store"
	"github.com/spf13/cobra"

	"github.com/danlafeir/em/internal/debug"
	"github.com/danlafeir/em/internal/github"
	"github.com/danlafeir/em/pkg/jira"
)

// JiraIssueRecord is a JIRA issue stored for a person under their review store.
type JiraIssueRecord struct {
	Key       string     `json:"key"`
	Summary   string     `json:"summary"`
	IssueType string     `json:"issue_type"`
	Status    string     `json:"status"`
	Priority  string     `json:"priority,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	URL       string     `json:"url"`
	Created   time.Time  `json:"created"`
	Completed *time.Time `json:"completed,omitempty"`
	FetchedAt time.Time  `json:"fetched_at"`
}

// GitHubPRRecord is a GitHub pull request stored for a person under their review store.
type GitHubPRRecord struct {
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	Repository string     `json:"repository"`
	URL        string     `json:"url"`
	State      string     `json:"state"`
	Role       string     `json:"role"` // "author" or "reviewer"
	CreatedAt  time.Time  `json:"created_at"`
	MergedAt   *time.Time `json:"merged_at,omitempty"`
	FetchedAt  time.Time  `json:"fetched_at"`
}

var (
	fetchFromFlag string
	fetchToFlag   string
)

var reviewFetchDataCmd = &cobra.Command{
	Use:   "fetch-data <name>",
	Short: "Fetch JIRA and GitHub data for a person",
	Long: `Fetch JIRA issues and GitHub pull requests for a configured person and store them locally.

Data is written to ~/.em/review/<name>/ as JSONL files.

Example:
  em review fetch-data "Alice Smith" --from 2026-01-01 --to 2026-06-01`,
	Args: cobra.ExactArgs(1),
	RunE: runReviewFetchData,
}

func init() {
	reviewFetchDataCmd.Flags().StringVar(&fetchFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	reviewFetchDataCmd.Flags().StringVar(&fetchToFlag, "to", "", "End date (YYYY-MM-DD)")
}

func runReviewFetchData(cmd *cobra.Command, args []string) error {
	slug := strings.ToLower(strings.TrimSpace(args[0]))

	people := getReviewPeople()
	var person *reviewPerson
	for i, p := range people {
		if p.slug == slug || strings.ToLower(p.displayName) == slug {
			person = &people[i]
			break
		}
	}
	if person == nil {
		return fmt.Errorf("person %q not found. Use 'em review add' to add them first", args[0])
	}

	var from, to time.Time
	var err error
	if fetchFromFlag != "" {
		if from, err = time.Parse("2006-01-02", fetchFromFlag); err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
	}
	if fetchToFlag != "" {
		if to, err = time.Parse("2006-01-02", fetchToFlag); err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
	}

	storeDir := filepath.Join(emConfigDir(), "review", person.slug)
	s, err := store.New(storeDir)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	jiraSchema := store.Open[JiraIssueRecord](s, "jira_issues")
	githubSchema := store.Open[GitHubPRRecord](s, "github_prs")

	ctx := context.Background()
	now := time.Now()

	fmt.Printf("Fetching JIRA issues for %s...\n", person.displayName)
	jiraClient, err := newReviewJiraClient()
	if err != nil {
		return fmt.Errorf("JIRA client: %w", err)
	}
	jiraRecords, err := fetchAndMapJiraIssues(ctx, jiraClient, person.jiraEmail, from, to, now)
	if err != nil {
		return fmt.Errorf("fetching JIRA issues: %w", err)
	}
	if err := jiraSchema.WriteAll(jiraRecords); err != nil {
		return fmt.Errorf("storing JIRA issues: %w", err)
	}
	fmt.Printf("  Stored %d JIRA issues → %s\n", len(jiraRecords), filepath.Join(s.Dir(), "jira_issues.jsonl"))

	fmt.Printf("Fetching GitHub PRs for %s...\n", person.displayName)
	ghClient, err := newReviewGithubClient()
	if err != nil {
		return fmt.Errorf("GitHub client: %w", err)
	}
	org := reviewConfigString("github.org")
	prRecords, err := fetchAndMapGithubPRs(ctx, ghClient, person.githubUsername, org, from, to, now)
	if err != nil {
		return fmt.Errorf("fetching GitHub PRs: %w", err)
	}
	if err := githubSchema.WriteAll(prRecords); err != nil {
		return fmt.Errorf("storing GitHub PRs: %w", err)
	}
	fmt.Printf("  Stored %d GitHub PRs → %s\n", len(prRecords), filepath.Join(s.Dir(), "github_prs.jsonl"))

	return nil
}

func fetchAndMapJiraIssues(ctx context.Context, client *jira.Client, email string, from, to time.Time, fetchedAt time.Time) ([]JiraIssueRecord, error) {
	jql := fmt.Sprintf(`assignee = "%s"`, email)
	if !from.IsZero() {
		jql += fmt.Sprintf(` AND updated >= "%s"`, from.Format("2006-01-02"))
	}
	if !to.IsZero() {
		jql += fmt.Sprintf(` AND updated <= "%s"`, to.Format("2006-01-02"))
	}

	issues, err := client.SearchAllIssues(ctx, jql, "", "")
	if err != nil {
		return nil, err
	}

	domain := reviewConfigString("jira.domain")
	records := make([]JiraIssueRecord, 0, len(issues))
	for _, issue := range issues {
		rec := JiraIssueRecord{
			Key:       issue.Key,
			Summary:   issue.Fields.Summary,
			IssueType: issue.Fields.IssueType.Name,
			Status:    issue.Fields.Status.Name,
			Labels:    issue.Fields.Labels,
			URL:       fmt.Sprintf("https://%s.atlassian.net/browse/%s", domain, issue.Key),
			Created:   issue.Fields.Created.Time,
			FetchedAt: fetchedAt,
		}
		if issue.Fields.Priority != nil {
			rec.Priority = issue.Fields.Priority.Name
		}
		if issue.Fields.ResolutionDate != nil && !issue.Fields.ResolutionDate.IsZero() {
			t := issue.Fields.ResolutionDate.Time
			rec.Completed = &t
		}
		records = append(records, rec)
	}
	return records, nil
}

func fetchAndMapGithubPRs(ctx context.Context, client *github.Client, username, org string, from, to time.Time, fetchedAt time.Time) ([]GitHubPRRecord, error) {
	dateFilter := buildGithubDateFilter(from, to)

	authorQuery := fmt.Sprintf("is:pr author:%s", username)
	if org != "" {
		authorQuery += fmt.Sprintf(" org:%s", org)
	}
	if dateFilter != "" {
		authorQuery += " " + dateFilter
	}

	authored, err := client.SearchPRs(ctx, authorQuery)
	if err != nil {
		return nil, fmt.Errorf("authored PRs: %w", err)
	}

	reviewerQuery := fmt.Sprintf("is:pr reviewed-by:%s", username)
	if org != "" {
		reviewerQuery += fmt.Sprintf(" org:%s", org)
	}
	if dateFilter != "" {
		reviewerQuery += " " + dateFilter
	}

	reviewed, err := client.SearchPRs(ctx, reviewerQuery)
	if err != nil {
		return nil, fmt.Errorf("reviewed PRs: %w", err)
	}

	var records []GitHubPRRecord
	for _, pr := range authored {
		records = append(records, prToRecord(pr, "author", fetchedAt))
	}

	authoredURLs := make(map[string]bool, len(authored))
	for _, pr := range authored {
		authoredURLs[pr.HTMLURL] = true
	}
	for _, pr := range reviewed {
		if !authoredURLs[pr.HTMLURL] {
			records = append(records, prToRecord(pr, "reviewer", fetchedAt))
		}
	}

	return records, nil
}

func buildGithubDateFilter(from, to time.Time) string {
	if from.IsZero() && to.IsZero() {
		return ""
	}
	if !from.IsZero() && !to.IsZero() {
		return fmt.Sprintf("created:%s..%s", from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	if !from.IsZero() {
		return fmt.Sprintf("created:>=%s", from.Format("2006-01-02"))
	}
	return fmt.Sprintf("created:<=%s", to.Format("2006-01-02"))
}

func prToRecord(pr github.PullRequest, role string, fetchedAt time.Time) GitHubPRRecord {
	var mergedAt *time.Time
	if pr.PullRequest != nil {
		mergedAt = pr.PullRequest.MergedAt
	}
	return GitHubPRRecord{
		Number:     pr.Number,
		Title:      pr.Title,
		Repository: repoFromURL(pr.RepositoryURL),
		URL:        pr.HTMLURL,
		State:      pr.State,
		Role:       role,
		CreatedAt:  pr.CreatedAt,
		MergedAt:   mergedAt,
		FetchedAt:  fetchedAt,
	}
}

// repoFromURL extracts "org/repo" from a GitHub API repository URL.
func repoFromURL(repositoryURL string) string {
	const prefix = "repos/"
	if idx := strings.Index(repositoryURL, prefix); idx >= 0 {
		return repositoryURL[idx+len(prefix):]
	}
	return repositoryURL
}

func reviewConfigString(key string) string {
	s, _ := getReviewConfigAny(key).(string)
	return s
}

func newReviewJiraClient() (*jira.Client, error) {
	config.InitConfig(emConfigDir()) //nolint:errcheck

	domain := reviewConfigString("jira.domain")
	email := reviewConfigString("jira.email")

	token, err := secrets.Read("jira", "api_token")
	if err != nil || token == "" {
		token = os.Getenv("JIRA_API_TOKEN")
	}

	if domain == "" {
		return nil, fmt.Errorf("JIRA domain not configured. Run: em config set jira.domain <domain>")
	}
	if email == "" {
		return nil, fmt.Errorf("JIRA email not configured. Run: em config set jira.email <email>")
	}
	if token == "" {
		return nil, fmt.Errorf("JIRA API token not configured. Run: em config set jira.api_token")
	}

	creds := jira.Credentials{
		Domain:   domain,
		Email:    email,
		APIToken: token,
	}
	if override := os.Getenv("JIRA_BASE_URL"); override != "" {
		creds.BaseURLOverride = override
	}
	return jira.NewClient(creds).WithDebug(debug.Enabled()), nil
}

func newReviewGithubClient() (*github.Client, error) {
	token, err := secrets.Read("github", "api_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("GitHub API token not configured. Run: em config set github.api_token")
	}
	creds := github.Credentials{
		Token: token,
		Org:   reviewConfigString("github.org"),
	}
	client, err := github.NewClient(creds)
	if err != nil {
		return nil, err
	}
	return client.WithDebug(debug.Enabled()), nil
}
