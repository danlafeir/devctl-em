package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"

	"github.com/danlafeir/em/pkg/httputil"
)

// Client is the GitHub API client backed by go-gh.
type Client struct {
	rest          *ghapi.RESTClient
	debug         bool
	dumpResponses bool
}

// WithDebug toggles per-request debug logging.
func (c *Client) WithDebug(v bool) *Client { c.debug = v; return c }

// WithDumpResponses toggles full response-body dumping. Off by default.
func (c *Client) WithDumpResponses(v bool) *Client { c.dumpResponses = v; return c }

// NewClient creates a new GitHub API client using go-gh.
func NewClient(creds Credentials) (*Client, error) {
	opts := ghapi.ClientOptions{
		AuthToken: creds.Token,
	}

	rest, err := ghapi.NewRESTClient(opts)
	if err != nil {
		return nil, err
	}

	return &Client{rest: rest}, nil
}

// NewClientWithTransport creates a GitHub API client with a custom HTTP transport.
// Intended for use in tests (e.g. pointing the client at an httptest.Server).
func NewClientWithTransport(creds Credentials, transport http.RoundTripper) (*Client, error) {
	opts := ghapi.ClientOptions{
		AuthToken: creds.Token,
		Transport: transport,
	}
	rest, err := ghapi.NewRESTClient(opts)
	if err != nil {
		return nil, err
	}
	return &Client{rest: rest}, nil
}

// linkNextRe matches rel="next" in Link headers.
var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// parseLinkHeader extracts the "next" URL from a GitHub Link header.
func parseLinkHeader(header string) string {
	if header == "" {
		return ""
	}
	matches := linkNextRe.FindStringSubmatch(header)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// doGet executes a GET request and returns the raw body and Link header value.
func (c *Client) doGet(ctx context.Context, path string) ([]byte, string, error) {
	start := time.Now()
	resp, err := c.rest.RequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		if c.debug {
			fmt.Fprintf(os.Stderr, "[debug] GitHub GET %s -> error: %v\n", path, err)
		}
		return nil, "", classifyGHError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading response: %w", err)
	}

	if c.debug {
		fmt.Fprintf(os.Stderr, "[debug] GitHub GET %s -> %d (%dms, %d bytes)\n",
			path, resp.StatusCode, time.Since(start).Milliseconds(), len(body))
	}
	if c.dumpResponses {
		fmt.Fprintf(os.Stderr, "[debug-upstream-response] GitHub GET %s\n%s\n",
			path, string(body))
	}

	return body, resp.Header.Get("Link"), nil
}

// classifyGHError converts go-gh's *api.HTTPError into a typed *httputil.APIError
// so callers and the cmd layer can branch on status code without string matching.
// Non-HTTP errors (network, JSON parse) pass through untouched.
func classifyGHError(err error) error {
	var httpErr *ghapi.HTTPError
	if errors.As(err, &httpErr) {
		return httputil.Classify(httputil.ProviderGitHub, httpErr.StatusCode, []byte(httpErr.Message))
	}
	return err
}

// doGetURL executes a GET against an absolute pagination URL by extracting the path.
func (c *Client) doGetURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("parsing pagination URL: %w", err)
	}
	path := strings.TrimPrefix(u.Path, "/")
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return c.doGet(ctx, path)
}

// ListTeamMembers lists members of a GitHub team, handling Link-header pagination.
func (c *Client) ListTeamMembers(ctx context.Context, org, teamSlug string) ([]User, error) {
	path := fmt.Sprintf("orgs/%s/teams/%s/members?per_page=100",
		url.PathEscape(org), url.PathEscape(teamSlug))

	body, link, err := c.doGet(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing team members: %w", err)
	}

	var members []User
	if err := json.Unmarshal(body, &members); err != nil {
		return nil, fmt.Errorf("parsing team members: %w", err)
	}

	nextURL := parseLinkHeader(link)
	for nextURL != "" {
		body, link, err = c.doGetURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("listing team members (pagination): %w", err)
		}
		var page []User
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing team members page: %w", err)
		}
		members = append(members, page...)
		nextURL = parseLinkHeader(link)
	}

	return members, nil
}

// ListTeamRepos lists repositories for a team, handling Link-header pagination.
func (c *Client) ListTeamRepos(ctx context.Context, org, teamSlug string) ([]Repository, error) {
	path := fmt.Sprintf("orgs/%s/teams/%s/repos?per_page=100",
		url.PathEscape(org), url.PathEscape(teamSlug))

	body, link, err := c.doGet(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing team repos: %w", err)
	}

	var repos []Repository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("parsing team repos: %w", err)
	}

	nextURL := parseLinkHeader(link)
	for nextURL != "" {
		body, link, err = c.doGetURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("listing team repos (pagination): %w", err)
		}
		var page []Repository
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing team repos page: %w", err)
		}
		repos = append(repos, page...)
		nextURL = parseLinkHeader(link)
	}

	return repos, nil
}

// ListWorkflows lists GitHub Actions workflows for a repository.
func (c *Client) ListWorkflows(ctx context.Context, owner, repo string) ([]Workflow, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/workflows",
		url.PathEscape(owner), url.PathEscape(repo))

	body, _, err := c.doGet(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}

	var resp WorkflowListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing workflows: %w", err)
	}

	return resp.Workflows, nil
}

// ListWorkflowRuns lists runs for a specific workflow, filtered by date range.
func (c *Client) ListWorkflowRuns(ctx context.Context, owner, repo string, workflowID int64, branch string, from, to time.Time) ([]WorkflowRun, error) {
	query := url.Values{}
	query.Set("status", "completed")
	query.Set("per_page", "100")
	if branch != "" {
		query.Set("branch", branch)
	}
	if !from.IsZero() || !to.IsZero() {
		var parts []string
		if !from.IsZero() {
			parts = append(parts, from.Format("2006-01-02"))
		}
		parts = append(parts, "..")
		if !to.IsZero() {
			parts = append(parts, to.Format("2006-01-02"))
		}
		query.Set("created", strings.Join(parts, ""))
	}

	path := fmt.Sprintf("repos/%s/%s/actions/workflows/%d/runs?%s",
		url.PathEscape(owner), url.PathEscape(repo), workflowID, query.Encode())

	var allRuns []WorkflowRun

	body, link, err := c.doGet(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing workflow runs: %w", err)
	}

	var resp WorkflowRunsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing workflow runs: %w", err)
	}
	allRuns = append(allRuns, resp.WorkflowRuns...)

	nextURL := parseLinkHeader(link)
	for nextURL != "" {
		body, link, err = c.doGetURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("listing workflow runs (pagination): %w", err)
		}
		var page WorkflowRunsResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing workflow runs page: %w", err)
		}
		allRuns = append(allRuns, page.WorkflowRuns...)
		nextURL = parseLinkHeader(link)
	}

	return allRuns, nil
}

// ListUserTeams lists teams for the authenticated user, filtered to a specific org.
func (c *Client) ListUserTeams(ctx context.Context, org string) ([]Team, error) {
	body, link, err := c.doGet(ctx, "user/teams?per_page=100")
	if err != nil {
		return nil, fmt.Errorf("listing user teams: %w", err)
	}

	var allTeams []Team
	if err := json.Unmarshal(body, &allTeams); err != nil {
		return nil, fmt.Errorf("parsing user teams: %w", err)
	}

	nextURL := parseLinkHeader(link)
	for nextURL != "" {
		body, link, err = c.doGetURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("listing user teams (pagination): %w", err)
		}
		var page []Team
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing user teams page: %w", err)
		}
		allTeams = append(allTeams, page...)
		nextURL = parseLinkHeader(link)
	}

	orgLower := strings.ToLower(org)
	var filtered []Team
	for _, t := range allTeams {
		if strings.ToLower(t.Organization.Login) == orgLower {
			filtered = append(filtered, t)
		}
	}

	return filtered, nil
}

// TestConnection verifies the GitHub credentials work.
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.WhoAmI(ctx)
	return err
}

// SearchPRs searches for pull requests using the GitHub search/issues API.
// query is a standard GitHub search query, e.g. "is:pr author:alice org:myorg".
// Pagination is handled automatically via Link headers.
func (c *Client) SearchPRs(ctx context.Context, query string) ([]PullRequest, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("per_page", "100")
	path := "search/issues?" + q.Encode()

	body, link, err := c.doGet(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("searching PRs: %w", err)
	}

	var result PRSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing PR search result: %w", err)
	}
	prs := result.Items

	nextURL := parseLinkHeader(link)
	for nextURL != "" {
		body, link, err = c.doGetURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("searching PRs (pagination): %w", err)
		}
		var page PRSearchResult
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing PR search page: %w", err)
		}
		prs = append(prs, page.Items...)
		nextURL = parseLinkHeader(link)
	}

	return prs, nil
}

// WhoAmI returns the authenticated GitHub user. Useful for printing a
// "Connected as ..." line after credentials are saved.
func (c *Client) WhoAmI(ctx context.Context) (*User, error) {
	body, _, err := c.doGet(ctx, "user")
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing /user response: %w", err)
	}
	return &u, nil
}
