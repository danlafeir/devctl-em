package snyk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danlafeir/em/pkg/httputil"
)

const apiVersion = "2025-11-05"

// Client is the main Snyk API client.
type Client struct {
	httpClient  httputil.HTTPDoer
	credentials Credentials
	rateLimiter *httputil.RateLimiter
}

// NewAuthClient creates a Snyk client with only token and site — no OrgID required.
// Use for operations that don't need an org (e.g. ListOrgs during config).
func NewAuthClient(token, site string) *Client {
	return NewClient(Credentials{Token: token, Site: site})
}

// NewClient creates a new Snyk API client.
func NewClient(creds Credentials) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		credentials: creds,
		rateLimiter: httputil.Default(),
	}
}

// NewClientWithHTTPDoer creates a Snyk client with a custom HTTP doer.
// Intended for use in tests (e.g. pointing the client at an httptest.Server).
func NewClientWithHTTPDoer(doer httputil.HTTPDoer, creds Credentials) *Client {
	return &Client{
		httpClient:  doer,
		credentials: creds,
		rateLimiter: &httputil.RateLimiter{MaxRetries: 0},
	}
}

// doRequest executes a request with authentication and rate limit handling.
// If fullURL is provided (non-empty), it is used as-is (for pagination next links).
// Otherwise, the request is built from path and query against the base URL.
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, fullURL string) ([]byte, error) {
	reqURL := fullURL
	if reqURL == "" {
		reqURL = c.credentials.BaseURL() + path
		if query == nil {
			query = url.Values{}
		}
		query.Set("version", apiVersion)
		reqURL += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.rateLimiter.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "token "+c.credentials.Token)
		req.Header.Set("Content-Type", "application/vnd.api+json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == 429 {
			lastErr = fmt.Errorf("rate limited (HTTP 429)")
			delay := c.rateLimiter.Backoff(attempt, resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Snyk API error %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return nil, fmt.Errorf("max retries exceeded")
}


// IsExploitable returns true for vulnerabilities with a known exploit.
// Snyk exploit maturity levels that qualify: "Proof of Concept", "Attacked".
func IsExploitable(maturity string) bool {
	switch maturity {
	case "Proof of Concept", "Attacked":
		return true
	}
	return false
}

// isExploitableDetails checks exploit maturity from a full exploitDetails value.
// It checks every entry in MaturityLevels (the current API field), falling back
// to the legacy scalar Maturity field when the array is empty.
func isExploitableDetails(d exploitDetails) bool {
	if len(d.MaturityLevels) > 0 {
		for _, ml := range d.MaturityLevels {
			if IsExploitable(ml.Level) {
				return true
			}
		}
		return false
	}
	return IsExploitable(d.Maturity)
}

// resolveMaturity returns a display string for the most relevant maturity level.
// It prefers the primary entry from MaturityLevels, then any entry, then the
// legacy Maturity scalar.
func resolveMaturity(d exploitDetails) string {
	for _, ml := range d.MaturityLevels {
		if ml.Type == "primary" {
			return ml.Level
		}
	}
	if len(d.MaturityLevels) > 0 {
		return d.MaturityLevels[0].Level
	}
	return d.Maturity
}

// adjustExploitableSeverity increments or decrements the exploitable count
// for the given severity level. delta should be +1 or -1.
func adjustExploitableSeverity(counts *OpenCounts, severity string, delta int) {
	switch severity {
	case "critical":
		counts.ExploitableCritical += delta
	case "high":
		counts.ExploitableHigh += delta
	case "medium":
		counts.ExploitableMedium += delta
	case "low":
		counts.ExploitableLow += delta
	}
}

// adjustSeverity increments the severity count and, when exploitable, the
// exploitable severity count for the given severity level.
func adjustSeverity(counts *OpenCounts, severity string, exploitable bool) {
	switch severity {
	case "critical":
		counts.Critical++
		if exploitable {
			counts.ExploitableCritical++
		}
	case "high":
		counts.High++
		if exploitable {
			counts.ExploitableHigh++
		}
	case "medium":
		counts.Medium++
		if exploitable {
			counts.ExploitableMedium++
		}
	case "low":
		counts.Low++
		if exploitable {
			counts.ExploitableLow++
		}
	}
}

// isFixable returns true if any coordinate has a fix available.
func isFixable(coords []coordinate) bool {
	for _, c := range coords {
		if c.IsFixableManually || c.IsFixableSnyk || c.IsFixableUpstream || c.IsPatchable || c.IsPinnable || c.IsUpgradeable {
			return true
		}
	}
	return false
}

// TestConnection verifies the Snyk credentials work.
func (c *Client) TestConnection(ctx context.Context) error {
	body, err := c.doRequest(ctx, "GET", "/rest/self", nil, "")
	if err != nil {
		return err
	}

	var resp selfResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parsing self response: %w", err)
	}
	return nil
}

// ListOrgs lists all Snyk organizations accessible to the authenticated user.
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	var all []Org

	query := url.Values{}
	query.Set("limit", "100")

	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", "/rest/orgs", query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("listing orgs: %w", err)
		}

		var resp orgListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing orgs: %w", err)
		}

		for _, d := range resp.Data {
			all = append(all, Org{ID: d.ID, Name: d.Attributes.Name})
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL != "" && nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}

	return all, nil
}

// GetProjectTargetMap returns a map of project ID → target ID for all projects in the org.
func (c *Client) GetProjectTargetMap(ctx context.Context) (map[string]string, error) {
	path := fmt.Sprintf("/rest/orgs/%s/projects", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")

	result := make(map[string]string)
	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("fetching projects: %w", err)
		}

		var resp projectListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing projects: %w", err)
		}

		for _, p := range resp.Data {
			result[p.ID] = p.Relationships.Target.Data.ID
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}
	return result, nil
}

// CountOpenIssues returns the current open issue counts broken down by severity,
// deduplicated by (target, title, severity) to match the Snyk UI's per-target grouping.
// Falls back to project-level deduplication if the projects endpoint is unavailable.
func (c *Client) CountOpenIssues(ctx context.Context, filter IssueFilter) (OpenCounts, error) {
	projectTargets, err := c.GetProjectTargetMap(ctx)
	if err != nil {
		// Projects endpoint unavailable — use project IDs directly as dedup keys.
		projectTargets = nil
	}

	path := fmt.Sprintf("/rest/orgs/%s/issues", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")
	if filter.ScanItemType != "" {
		query.Set("scan_item.type", filter.ScanItemType)
	}

	type issueKey struct {
		targetID string
		title    string
		severity string
	}
	// ignoredRecord stores what was counted when a key was first seen as ignored,
	// so it can be reversed if a non-ignored instance of the same key appears later.
	type ignoredRecord struct {
		fixable     bool
		exploitable bool
		severity    string
	}
	seenOpen    := make(map[issueKey]struct{})
	seenIgnored := make(map[issueKey]ignoredRecord)

	var counts OpenCounts
	nextURL := ""
	for {
		var body []byte
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return OpenCounts{}, fmt.Errorf("counting open issues: %w", err)
		}

		var resp issueListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return OpenCounts{}, fmt.Errorf("counting open issues: %w", err)
		}

		for _, d := range resp.Data {
			if d.Attributes.Status != "open" {
				continue
			}
			projectID := d.Relationships.ScanItem.Data.ID
			targetID := projectTargets[projectID]
			if targetID == "" {
				targetID = projectID
			}
			key := issueKey{
				targetID: targetID,
				title:    d.Attributes.Title,
				severity: strings.ToLower(d.Attributes.EffectiveSeverityLevel),
			}
			if d.Attributes.Ignored {
				// Skip if already counted in either bucket — non-ignored takes precedence.
				if _, ok := seenOpen[key]; ok {
					continue
				}
				if _, alreadySeen := seenIgnored[key]; alreadySeen {
					continue
				}
				exploitable := isExploitableDetails(d.Attributes.ExploitDetails)
				fixable := isFixable(d.Attributes.Coordinates)
				seenIgnored[key] = ignoredRecord{fixable: fixable, exploitable: exploitable, severity: key.severity}
				counts.Ignored++
				if fixable {
					counts.IgnoredFixable++
					if exploitable {
						counts.ExploitableIgnoredFixable++
					}
				} else {
					counts.IgnoredUnfixable++
					if exploitable {
						counts.ExploitableIgnoredUnfixable++
					}
				}
				if exploitable {
					adjustExploitableSeverity(&counts, key.severity, 1)
				}
				continue
			}
			if _, ok := seenOpen[key]; ok {
				continue
			}
			// If this key was previously counted as ignored, undo those counts first.
			if rec, wasIgnored := seenIgnored[key]; wasIgnored {
				delete(seenIgnored, key)
				counts.Ignored--
				if rec.fixable {
					counts.IgnoredFixable--
					if rec.exploitable {
						counts.ExploitableIgnoredFixable--
					}
				} else {
					counts.IgnoredUnfixable--
					if rec.exploitable {
						counts.ExploitableIgnoredUnfixable--
					}
				}
				if rec.exploitable {
					adjustExploitableSeverity(&counts, rec.severity, -1)
				}
			}
			seenOpen[key] = struct{}{}
			counts.Total++
			fixable := isFixable(d.Attributes.Coordinates)
			exploitable := isExploitableDetails(d.Attributes.ExploitDetails)
			if fixable {
				counts.Fixable++
				if exploitable {
					counts.ExploitableFixable++
				}
			} else {
				counts.Unfixable++
				if exploitable {
					counts.ExploitableUnfixable++
				}
			}
			adjustSeverity(&counts, key.severity, exploitable)
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}
	return counts, nil
}

// ListResolvedIssues fetches issues resolved within the given date range.
// It filters by update time to catch recently resolved issues, then returns
// only those with status "resolved" and a resolved_at within the range.
func (c *Client) ListResolvedIssues(ctx context.Context, from, to time.Time, filter IssueFilter) ([]Issue, error) {
	var all []Issue
	path := fmt.Sprintf("/rest/orgs/%s/issues", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")
	query.Set("updated_after", from.Format(time.RFC3339))
	query.Set("updated_before", to.Format(time.RFC3339))
	if filter.ScanItemType != "" {
		query.Set("scan_item.type", filter.ScanItemType)
	}

	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("listing resolved issues: %w", err)
		}

		var resp issueListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing resolved issues: %w", err)
		}

		for _, d := range resp.Data {
			if d.Attributes.Status != "resolved" {
				continue
			}
			issue := Issue{
				ID:             d.ID,
				Title:          d.Attributes.Title,
				Severity:       d.Attributes.EffectiveSeverityLevel,
				IssueType:      d.Attributes.Type,
				Status:         d.Attributes.Status,
				IsFixable:      isFixable(d.Attributes.Coordinates),
				Exploitability: resolveMaturity(d.Attributes.ExploitDetails),
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
				issue.CreatedAt = t
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.ResolvedAt); err == nil {
				issue.ResolvedAt = t
			}
			// Only include if resolved_at falls within the requested range
			if issue.ResolvedAt.IsZero() || issue.ResolvedAt.Before(from) || issue.ResolvedAt.After(to) {
				continue
			}
			all = append(all, issue)
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL != "" && nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}

	return all, nil
}

// ListIssues fetches all issues for the org within the given date range.
func (c *Client) ListIssues(ctx context.Context, from, to time.Time, filter IssueFilter) ([]Issue, error) {
	var all []Issue
	path := fmt.Sprintf("/rest/orgs/%s/issues", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")
	query.Set("created_after", from.Format(time.RFC3339))
	query.Set("created_before", to.Format(time.RFC3339))
	if filter.ScanItemType != "" {
		query.Set("scan_item.type", filter.ScanItemType)
	}

	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("listing issues: %w", err)
		}

		var resp issueListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing issues: %w", err)
		}

		for _, d := range resp.Data {
			issue := Issue{
				ID:             d.ID,
				Title:          d.Attributes.Title,
				Severity:       d.Attributes.EffectiveSeverityLevel,
				IssueType:      d.Attributes.Type,
				Status:         d.Attributes.Status,
				IsFixable:      isFixable(d.Attributes.Coordinates),
				IsIgnored:      d.Attributes.Ignored,
				Exploitability: resolveMaturity(d.Attributes.ExploitDetails),
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
				issue.CreatedAt = t
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.ResolvedAt); err == nil {
				issue.ResolvedAt = t
			}
			all = append(all, issue)
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL != "" && nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}

	return all, nil
}
