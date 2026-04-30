package httputil

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestClassifyHints(t *testing.T) {
	cases := []struct {
		provider Provider
		status   int
		// substrings the hint MUST contain
		mustContain []string
	}{
		{ProviderJIRA, 401, []string{"token invalid", "em metrics jira config"}},
		{ProviderJIRA, 403, []string{"lacks read access", "JIRA"}},
		{ProviderJIRA, 404, []string{"404", "project key"}},
		{ProviderJIRA, 503, []string{"transient", "JIRA"}},

		{ProviderGitHub, 401, []string{"token invalid", "em metrics github config"}},
		{ProviderGitHub, 403, []string{"`repo` scope", "SSO"}},
		{ProviderGitHub, 404, []string{"404", "org"}},
		{ProviderGitHub, 502, []string{"transient", "GitHub"}},

		{ProviderSnyk, 401, []string{"token invalid", "em metrics snyk config"}},
		{ProviderSnyk, 403, []string{"Snyk org"}},
		{ProviderSnyk, 404, []string{"404", "org ID"}},
		{ProviderSnyk, 500, []string{"transient", "Snyk"}},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("%s/%d", tc.provider, tc.status)
		t.Run(name, func(t *testing.T) {
			err := Classify(tc.provider, tc.status, []byte("body"))
			msg := err.Error()
			for _, want := range tc.mustContain {
				if !strings.Contains(msg, want) {
					t.Errorf("expected error to contain %q, got %q", want, msg)
				}
			}
			if err.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", err.StatusCode, tc.status)
			}
			if err.Provider != tc.provider {
				t.Errorf("Provider = %q, want %q", err.Provider, tc.provider)
			}
		})
	}
}

func TestClassifyUnknownStatusFallsBack(t *testing.T) {
	err := Classify(ProviderJIRA, 418, []byte("teapot"))
	msg := err.Error()
	// No curated hint, but the status and body should still be present.
	if !strings.Contains(msg, "418") {
		t.Errorf("expected status 418 in error, got %q", msg)
	}
	if !strings.Contains(msg, "teapot") {
		t.Errorf("expected body excerpt in error, got %q", msg)
	}
}

func TestIsAPIError(t *testing.T) {
	err := Classify(ProviderJIRA, 401, nil)
	if !IsAPIError(err) {
		t.Fatal("expected IsAPIError to be true for *APIError")
	}
	wrapped := fmt.Errorf("connecting: %w", err)
	if !IsAPIError(wrapped) {
		t.Fatal("expected IsAPIError to be true for wrapped *APIError")
	}
	if IsAPIError(errors.New("plain")) {
		t.Fatal("expected IsAPIError to be false for plain error")
	}
}

func TestErrorsAsRecoversAPIError(t *testing.T) {
	wrapped := fmt.Errorf("calling JIRA: %w", Classify(ProviderJIRA, 403, []byte("nope")))
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As did not recover *APIError")
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want 403", apiErr.StatusCode)
	}
}

func TestBodyExcerptTruncates(t *testing.T) {
	long := strings.Repeat("a", 500)
	err := Classify(ProviderJIRA, 500, []byte(long))
	msg := err.Error()
	if !strings.Contains(msg, "…") {
		t.Errorf("expected truncation marker in long-body error, got %q", msg)
	}
	if len(msg) > 600 {
		t.Errorf("error string too long: %d chars", len(msg))
	}
}
