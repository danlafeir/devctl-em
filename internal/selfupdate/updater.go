package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const Repo = "danlafeir/em"

// Release is a minimal representation of a GitHub Release API response.
type Release struct {
	TagName    string  `json:"tag_name"`
	PreRelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset is a single downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LatestRelease fetches the latest published GitHub Release for em.
// Returns nil, nil when no releases have been published yet.
func LatestRelease() (*Release, error) {
	return LatestReleaseFor(Repo)
}

// LatestReleaseFor fetches the latest release for the given "owner/repo".
// Separated for testing.
func LatestReleaseFor(repo string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, nil // no releases yet
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("parsing release response: %w", err)
	}
	return &r, nil
}

// LatestPreRelease fetches the most recent pre-release for em.
// Returns nil, nil when no pre-releases have been published yet.
func LatestPreRelease() (*Release, error) {
	return LatestPreReleaseFor(Repo)
}

// LatestPreReleaseFor fetches the most recent pre-release for the given "owner/repo".
// Separated for testing.
func LatestPreReleaseFor(repo string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", repo)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parsing releases response: %w", err)
	}
	for _, r := range releases {
		if r.PreRelease {
			r := r
			return &r, nil
		}
	}
	return nil, nil // no pre-releases yet
}

// IsNewer reports whether tagName is a newer release than currentVersion.
//
// Both values may be "vX.Y.Z", "vX.Y.Z-N-gHASH" (git describe output), or a
// bare commit hash. A bare hash is treated as older than any semver tag so
// that users on pre-release dev builds always receive an upgrade prompt.
func IsNewer(tagName, currentVersion string) bool {
	if tagName == currentVersion {
		return false
	}
	a := parseSemver(tagName)
	b := parseSemver(currentVersion)
	if a == nil {
		return false // can't parse the remote tag — don't prompt
	}
	if b == nil {
		return true // current is a hash/dev build — any tagged release is newer
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parseSemver(s string) []int {
	s = strings.TrimPrefix(s, "v")
	// strip pre-release / git-describe suffix: "1.2.3-4-gabcdef" → "1.2.3"
	s = strings.SplitN(s, "-", 2)[0]
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

// AssetURL returns the download URL for the binary matching the current OS and
// architecture, or an empty string if no matching asset was found.
func AssetURL(assets []Asset) string {
	want := fmt.Sprintf("em-%s-%s", runtime.GOOS, runtime.GOARCH)
	for _, a := range assets {
		if a.Name == want {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// RunWithChannel checks for a newer release in the given channel ("stable" or
// "beta") and, if one exists, downloads it and replaces the running binary.
func RunWithChannel(currentVersion, channel string, cmd *cobra.Command) {
	var (
		release *Release
		err     error
	)
	if channel == "beta" {
		release, err = LatestPreRelease()
	} else {
		release, err = LatestRelease()
	}
	if err != nil {
		cmd.PrintErrf("Failed to check for updates: %v\n", err)
		return
	}
	if release == nil {
		cmd.Printf("No %s releases published yet.\n", channel)
		return
	}

	cmd.Printf("Current version: %s\n", currentVersion)
	cmd.Printf("Latest %s:  %s\n", channel, release.TagName)

	// Beta channel: update whenever the tag differs, catching beta.1 → beta.2.
	// Stable channel: use semver comparison so downgrades are not offered.
	if channel == "beta" {
		if release.TagName == currentVersion {
			cmd.Println("Already up to date.")
			return
		}
	} else {
		if !IsNewer(release.TagName, currentVersion) {
			cmd.Println("Already up to date.")
			return
		}
	}

	installRelease(release, cmd)
}

// Run checks for a newer stable release and, if one exists, downloads it and
// replaces the running binary. Called by the `em update` command.
func Run(currentVersion string, cmd *cobra.Command) {
	RunWithChannel(currentVersion, "stable", cmd)
}

// installRelease downloads and installs the binary from the given release.
func installRelease(release *Release, cmd *cobra.Command) {
	url := AssetURL(release.Assets)
	if url == "" {
		cmd.PrintErrf("No binary found for %s/%s in release %s.\n",
			runtime.GOOS, runtime.GOARCH, release.TagName)
		return
	}

	cmd.Printf("Downloading %s...\n", url)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		cmd.PrintErrf("Download failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		cmd.PrintErrf("Download failed: HTTP %d\n", resp.StatusCode)
		return
	}

	tmp, err := os.CreateTemp("", "em-update-*")
	if err != nil {
		cmd.PrintErrf("Failed to create temp file: %v\n", err)
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		cmd.PrintErrf("Failed to write update: %v\n", err)
		return
	}
	tmp.Close()

	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		cmd.PrintErrf("Failed to set permissions: %v\n", err)
		return
	}

	self, err := os.Executable()
	if err != nil {
		cmd.PrintErrln("Could not determine current binary path.")
		return
	}

	if err := os.Rename(tmp.Name(), self); err != nil {
		cmd.Println("Permission denied, retrying with sudo...")
		mv := exec.Command("sudo", "mv", tmp.Name(), self)
		mv.Stdin = os.Stdin
		mv.Stdout = os.Stdout
		mv.Stderr = os.Stderr
		if err := mv.Run(); err != nil {
			cmd.PrintErrf("Failed to install update: %v\n", err)
			return
		}
	}

	cmd.Printf("Updated to %s.\n", release.TagName)
}
