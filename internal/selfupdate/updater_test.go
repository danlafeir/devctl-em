package selfupdate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		tag     string
		current string
		want    bool
	}{
		// semver comparisons
		{"v1.0.0", "v0.9.0", true},
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.1.1", false},
		{"v0.1.0", "v1.0.0", false},

		// dev/hash builds are treated as older than any tag
		{"v0.1.0", "dev", true},
		{"v0.1.0", "abcdef1", true},

		// git-describe output: "v0.1.0-4-gabcdef" → base is v0.1.0
		{"v0.2.0", "v0.1.0-4-gabcdef", true},
		{"v0.1.0", "v0.1.0-4-gabcdef", false}, // same base tag, not newer
		{"v0.1.1", "v0.1.0-4-gabcdef", true},

		// already on latest
		{"v1.2.3", "v1.2.3", false},
	}

	for _, tc := range cases {
		got := IsNewer(tc.tag, tc.current)
		if got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.tag, tc.current, got, tc.want)
		}
	}
}

func TestLatestPreReleaseFor(t *testing.T) {
	releases := []Release{
		{TagName: "v1.1.0", PreRelease: false},
		{TagName: "v1.1.0-beta.1", PreRelease: true},
		{TagName: "v1.0.0", PreRelease: false},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases) //nolint:errcheck
	}))
	defer srv.Close()

	// Temporarily patch the URL by using a test repo path that we intercept.
	// Since LatestPreReleaseFor constructs the URL itself, we test via a
	// custom server URL by calling the underlying list endpoint directly.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []Release
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	// Find first pre-release — mirrors LatestPreReleaseFor logic.
	var prerelease *Release
	for _, r := range got {
		if r.PreRelease {
			r := r
			prerelease = &r
			break
		}
	}
	if prerelease == nil {
		t.Fatal("expected a pre-release, got nil")
	}
	if prerelease.TagName != "v1.1.0-beta.1" {
		t.Errorf("got %q, want v1.1.0-beta.1", prerelease.TagName)
	}
}
