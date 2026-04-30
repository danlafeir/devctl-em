package selfupdate

import "testing"

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
