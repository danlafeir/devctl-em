package httputil

import (
	"errors"
	"fmt"
	"strings"
)

// Provider names a supported upstream API.
type Provider string

const (
	ProviderJIRA   Provider = "JIRA"
	ProviderGitHub Provider = "GitHub"
	ProviderSnyk   Provider = "Snyk"
)

// APIError is a typed error carrying an upstream HTTP status code along with a
// provider-specific, user-actionable hint. Clients construct one via Classify
// when an upstream returns a non-2xx response; callers can recover the status
// code and provider via errors.As.
type APIError struct {
	Provider   Provider
	StatusCode int
	Body       string
	Hint       string
}

func (e *APIError) Error() string {
	if e.Hint != "" {
		if excerpt := bodyExcerpt(e.Body); excerpt != "" {
			return fmt.Sprintf("%s %d: %s (%s)", e.Provider, e.StatusCode, e.Hint, excerpt)
		}
		return fmt.Sprintf("%s %d: %s", e.Provider, e.StatusCode, e.Hint)
	}
	return fmt.Sprintf("%s API error %d: %s", e.Provider, e.StatusCode, bodyExcerpt(e.Body))
}

// Classify produces an APIError with provider-specific guidance for the given
// status code. Status codes outside the curated set get a generic hint so the
// caller can pass through unknown failure modes without losing information.
func Classify(provider Provider, statusCode int, body []byte) *APIError {
	return &APIError{
		Provider:   provider,
		StatusCode: statusCode,
		Body:       string(body),
		Hint:       hintFor(provider, statusCode),
	}
}

// IsAPIError reports whether err (or anything it wraps) is an *APIError.
func IsAPIError(err error) bool {
	var target *APIError
	return errors.As(err, &target)
}

func hintFor(provider Provider, status int) string {
	switch {
	case status == 401:
		return "API token invalid, expired, or revoked. Re-run `em metrics " + configCommandFor(provider) + " config` to set a fresh token."
	case status == 403:
		return forbiddenHint(provider)
	case status == 404:
		return notFoundHint(provider)
	case status >= 500 && status < 600:
		return fmt.Sprintf("Upstream %s server error. This is usually transient — retry in a few minutes.", provider)
	}
	return ""
}

func configCommandFor(p Provider) string {
	switch p {
	case ProviderJIRA:
		return "jira"
	case ProviderGitHub:
		return "github"
	case ProviderSnyk:
		return "snyk"
	}
	return string(p)
}

func forbiddenHint(p Provider) string {
	switch p {
	case ProviderJIRA:
		return "Your account or token lacks read access to this project. Open the project in a browser to verify it's visible to your JIRA user, then re-check `jira.project` / `jira.jql_filter_for_metrics`."
	case ProviderGitHub:
		return "Token is missing the `repo` scope, or you don't have access to this org/team. SSO-protected orgs require token authorization at https://github.com/orgs/<org>/sso."
	case ProviderSnyk:
		return "Token lacks access to this Snyk org. Verify the org ID and that this token belongs to a member of that org."
	}
	return "Permission denied. Check that your token has the required scopes and access."
}

func notFoundHint(p Provider) string {
	switch p {
	case ProviderJIRA:
		return "JIRA returned 404 — the project key, JQL, or issue may not exist or be visible to this token. Check `jira.project` / `jira.jql_filter_for_metrics`."
	case ProviderGitHub:
		return "GitHub returned 404 — org, team slug, repo, or workflow name is wrong, or the token can't see it (private repos without `repo` scope return 404, not 403)."
	case ProviderSnyk:
		return "Snyk returned 404 — the org ID is wrong or the token can't see it. Re-run `em metrics snyk config` to pick the right org."
	}
	return "Resource not found. Verify the identifier is correct and your token can see it."
}

// bodyExcerpt trims and shortens a response body for inclusion in error
// messages. Empty for empty bodies.
func bodyExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	const max = 240
	if len(body) > max {
		body = body[:max] + "…"
	}
	return body
}
