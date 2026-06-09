package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danlafeir/em/pkg/httputil"
)

// Client is the main JIRA Cloud API client.
type Client struct {
	httpClient    httputil.HTTPDoer
	credentials   Credentials
	rateLimiter   *httputil.RateLimiter
	debug         bool
	dumpResponses bool
}

// WithDebug toggles per-request debug logging (method, URL, status, duration).
func (c *Client) WithDebug(v bool) *Client { c.debug = v; return c }

// WithDumpResponses toggles full response-body dumping. Off by default; only
// enable on commands that explicitly opt in via --debug-upstream-response.
func (c *Client) WithDumpResponses(v bool) *Client { c.dumpResponses = v; return c }

// NewClient creates a new JIRA Cloud API client.
func NewClient(creds Credentials) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		credentials: creds,
		rateLimiter: httputil.Default(),
	}
}

// BaseURL returns the JIRA instance base URL.
func (c *Client) BaseURL() string {
	return c.credentials.BaseURL()
}

// BrowseURL returns the URL to view an issue in the JIRA UI.
func (c *Client) BrowseURL(issueKey string) string {
	return c.credentials.BaseURL() + "/browse/" + issueKey
}

// doRequest executes a request with authentication, rate limit handling, and optional body.
func (c *Client) doRequest(ctx context.Context, method, fullURL string, body []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.rateLimiter.MaxRetries; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		auth := c.credentials.Email + ":" + c.credentials.APIToken
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		req.Header.Set("Authorization", "Basic "+encoded)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		start := time.Now()
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if c.debug {
			fmt.Fprintf(os.Stderr, "[debug] JIRA %s %s -> %d (%dms, %d bytes)\n",
				method, fullURL, resp.StatusCode, time.Since(start).Milliseconds(), len(respBody))
		}
		if c.dumpResponses {
			fmt.Fprintf(os.Stderr, "[debug-upstream-response] JIRA %s %s\n%s\n",
				method, fullURL, string(respBody))
		}

		if resp.StatusCode == 429 {
			lastErr = httputil.Classify(httputil.ProviderJIRA, resp.StatusCode, respBody)
			delay := c.rateLimiter.Backoff(attempt, resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			excerpt := respBody
			var errResp ErrorResponse
			if json.Unmarshal(respBody, &errResp) == nil && (len(errResp.ErrorMessages) > 0 || len(errResp.Errors) > 0) {
				msgs := errResp.ErrorMessages
				for k, v := range errResp.Errors {
					msgs = append(msgs, fmt.Sprintf("%s: %s", k, v))
				}
				excerpt = []byte(strings.Join(msgs, "; "))
			}
			return nil, httputil.Classify(httputil.ProviderJIRA, resp.StatusCode, excerpt)
		}

		return respBody, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// httpAdapter executes a GET-style request (no body) with authentication and rate limit handling.
func (c *Client) httpAdapter(ctx context.Context, method, path string, query url.Values) ([]byte, error) {
	fullURL := c.credentials.BaseURL() + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}
	return c.doRequest(ctx, method, fullURL, nil)
}

// SearchUsers searches for JIRA users by display name or email.
// Returns only active users with a known email address.
func (c *Client) SearchUsers(ctx context.Context, query string) ([]User, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("maxResults", "10")

	data, err := c.httpAdapter(ctx, "GET", "/rest/api/3/user/search", q)
	if err != nil {
		return nil, fmt.Errorf("searching users: %w", err)
	}

	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parsing user search result: %w", err)
	}

	var active []User
	for _, u := range users {
		if u.Active && u.EmailAddress != "" {
			active = append(active, u)
		}
	}
	return active, nil
}

// UpdateIssueFields updates specific fields on a JIRA issue via PUT.
func (c *Client) UpdateIssueFields(ctx context.Context, issueKey string, fields map[string]any) error {
	payload := map[string]any{"fields": fields}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling update payload: %w", err)
	}
	fullURL := c.credentials.BaseURL() + fmt.Sprintf("/rest/api/3/issue/%s", issueKey)
	_, err = c.doRequest(ctx, "PUT", fullURL, body)
	return err
}


// SearchIssues performs a JQL search with pagination.
func (c *Client) SearchIssues(ctx context.Context, jql string, opts SearchOptions) (*SearchResult, error) {
	query := url.Values{}
	query.Set("jql", jql)
	query.Set("startAt", strconv.Itoa(opts.StartAt))
	if opts.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(opts.MaxResults))
	} else {
		query.Set("maxResults", "100")
	}
	if opts.Fields != "" {
		query.Set("fields", opts.Fields)
	}
	if opts.Expand != "" {
		query.Set("expand", opts.Expand)
	}

	data, err := c.httpAdapter(ctx, "GET", "/rest/api/3/search/jql", query)
	if err != nil {
		return nil, err
	}

	var result SearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing search result: %w", err)
	}

	return &result, nil
}

// SearchAllIssues fetches all issues matching the JQL query, handling pagination.
func (c *Client) SearchAllIssues(ctx context.Context, jql string, fields string, expand string) ([]Issue, error) {
	var allIssues []Issue
	startAt := 0
	maxResults := 100

	for {
		result, err := c.SearchIssues(ctx, jql, SearchOptions{
			StartAt:    startAt,
			MaxResults: maxResults,
			Fields:     fields,
			Expand:     expand,
		})
		if err != nil {
			return nil, err
		}

		allIssues = append(allIssues, result.Issues...)

		// Check if we've fetched all issues
		if startAt+len(result.Issues) >= result.Total {
			break
		}
		startAt += len(result.Issues)
	}

	return allIssues, nil
}

// GetIssueChangelog fetches the changelog for a specific issue.
func (c *Client) GetIssueChangelog(ctx context.Context, issueKey string, startAt int) (*ChangelogResult, error) {
	query := url.Values{}
	query.Set("startAt", strconv.Itoa(startAt))
	query.Set("maxResults", "100")

	path := fmt.Sprintf("/rest/api/3/issue/%s/changelog", issueKey)
	data, err := c.httpAdapter(ctx, "GET", path, query)
	if err != nil {
		return nil, err
	}

	var result ChangelogResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing changelog: %w", err)
	}

	return &result, nil
}

// GetFullChangelog fetches all changelog entries for an issue, handling pagination.
func (c *Client) GetFullChangelog(ctx context.Context, issueKey string) ([]ChangelogEntry, error) {
	var allEntries []ChangelogEntry
	startAt := 0

	for {
		result, err := c.GetIssueChangelog(ctx, issueKey, startAt)
		if err != nil {
			return nil, err
		}

		allEntries = append(allEntries, result.Values...)

		if startAt+len(result.Values) >= result.Total {
			break
		}
		startAt += len(result.Values)
	}

	return allEntries, nil
}

// GetIssueWithChangelog fetches an issue with its full changelog.
func (c *Client) GetIssueWithChangelog(ctx context.Context, issueKey string) (*IssueWithHistory, error) {
	// First fetch the issue
	result, err := c.SearchIssues(ctx, fmt.Sprintf("key = %s", issueKey), SearchOptions{
		MaxResults: 1,
		Fields:     "*all",
	})
	if err != nil {
		return nil, err
	}
	if len(result.Issues) == 0 {
		return nil, fmt.Errorf("issue not found: %s", issueKey)
	}

	issue := result.Issues[0]

	// Fetch full changelog
	changelog, err := c.GetFullChangelog(ctx, issueKey)
	if err != nil {
		return nil, err
	}

	// Extract status transitions
	transitions := ExtractStatusTransitions(changelog)

	return &IssueWithHistory{
		Issue:       issue,
		Transitions: transitions,
	}, nil
}

// ExtractStatusTransitions parses changelog entries to find status changes.
func ExtractStatusTransitions(entries []ChangelogEntry) []StatusTransition {
	var transitions []StatusTransition

	for _, entry := range entries {
		for _, item := range entry.Items {
			if item.Field == "status" {
				transitions = append(transitions, StatusTransition{
					Timestamp:  entry.Created.Time,
					FromStatus: item.FromString,
					ToStatus:   item.ToString,
				})
			}
		}
	}

	return transitions
}

// FetchIssuesWithHistory fetches all issues matching JQL and their changelogs.
// This is the main entry point for metrics calculations.
func (c *Client) FetchIssuesWithHistory(ctx context.Context, jql string, progressFn func(current, total int)) ([]IssueWithHistory, error) {
	// First fetch all issues with changelog expansion
	issues, err := c.SearchAllIssues(ctx, jql, "*all", "changelog")
	if err != nil {
		return nil, err
	}

	result := make([]IssueWithHistory, 0, len(issues))

	for i, issue := range issues {
		if progressFn != nil {
			progressFn(i+1, len(issues))
		}

		var transitions []StatusTransition

		// If changelog was included in expansion, use it
		if issue.Changelog != nil {
			transitions = ExtractStatusTransitions(issue.Changelog.Histories)

			// If changelog was paginated, fetch the rest
			if issue.Changelog.Total > len(issue.Changelog.Histories) {
				moreEntries, err := c.GetFullChangelog(ctx, issue.Key)
				if err != nil {
					return nil, fmt.Errorf("fetching changelog for %s: %w", issue.Key, err)
				}
				transitions = ExtractStatusTransitions(moreEntries)
			}
		} else {
			// Fetch changelog separately
			entries, err := c.GetFullChangelog(ctx, issue.Key)
			if err != nil {
				return nil, fmt.Errorf("fetching changelog for %s: %w", issue.Key, err)
			}
			transitions = ExtractStatusTransitions(entries)
		}

		result = append(result, IssueWithHistory{
			Issue:       issue,
			Transitions: transitions,
		})
	}

	return result, nil
}

// ListBoards returns all agile boards for the given project key.
// It resolves the project key to a numeric ID and uses the projectLocation
// parameter, which is more reliable than projectKeyOrId for finding boards.
func (c *Client) ListBoards(ctx context.Context, projectKey string) ([]Board, error) {
	// Resolve project key to numeric ID
	projectID, err := c.getProjectID(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("resolving project key: %w", err)
	}

	var allBoards []Board
	startAt := 0

	for {
		query := url.Values{}
		query.Set("projectLocation", projectID)
		query.Set("startAt", strconv.Itoa(startAt))

		data, err := c.httpAdapter(ctx, "GET", "/rest/agile/1.0/board", query)
		if err != nil {
			return nil, fmt.Errorf("listing boards: %w", err)
		}

		var result BoardListResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("parsing board list: %w", err)
		}

		allBoards = append(allBoards, result.Values...)

		if result.IsLast || len(result.Values) == 0 {
			break
		}
		startAt += len(result.Values)
	}

	return allBoards, nil
}

// getProjectID resolves a project key to its numeric ID.
func (c *Client) getProjectID(ctx context.Context, projectKey string) (string, error) {
	path := fmt.Sprintf("/rest/api/3/project/%s", projectKey)
	data, err := c.httpAdapter(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}

	var proj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &proj); err != nil {
		return "", fmt.Errorf("parsing project: %w", err)
	}

	return proj.ID, nil
}

// GetBoardConfiguration returns the configuration for a board, including its filter.
func (c *Client) GetBoardConfiguration(ctx context.Context, boardID int) (*BoardConfig, error) {
	path := fmt.Sprintf("/rest/agile/1.0/board/%d/configuration", boardID)
	data, err := c.httpAdapter(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting board configuration: %w", err)
	}

	var config BoardConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing board configuration: %w", err)
	}

	return &config, nil
}

// GetFilter returns a JIRA saved filter by ID.
func (c *Client) GetFilter(ctx context.Context, filterID string) (*Filter, error) {
	path := fmt.Sprintf("/rest/api/3/filter/%s", filterID)
	data, err := c.httpAdapter(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting filter: %w", err)
	}

	var filter Filter
	if err := json.Unmarshal(data, &filter); err != nil {
		return nil, fmt.Errorf("parsing filter: %w", err)
	}

	return &filter, nil
}

// TestConnection verifies the JIRA credentials work.
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.httpAdapter(ctx, "GET", "/rest/api/3/myself", nil)
	return err
}

// WhoAmI returns the authenticated JIRA user. Useful for printing a
// "Connected as ..." line after credentials are saved.
func (c *Client) WhoAmI(ctx context.Context) (*User, error) {
	body, err := c.httpAdapter(ctx, "GET", "/rest/api/3/myself", nil)
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing /myself response: %w", err)
	}
	return &u, nil
}
