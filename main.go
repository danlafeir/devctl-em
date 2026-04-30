/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danlafeir/em/cmd"
	"github.com/danlafeir/em/internal/selfupdate"
	"github.com/danlafeir/cli-go/pkg/secrets"
)

// Build metadata is set at build time via -ldflags. See Makefile.
var (
	BuildVersion = "dev"
	BuildGitHash = "dev"
	BuildDate    = "unknown"
)

func checkUpgrade() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	checkFile := filepath.Join(home, ".em", "upgrade-check")
	os.MkdirAll(filepath.Dir(checkFile), 0o755)

	today := time.Now().Format("2006-01-02")
	var lastDate string
	if f, err := os.Open(checkFile); err == nil {
		fmt.Fscanf(f, "%s", &lastDate)
		f.Close()
	}
	if lastDate == today {
		return // already checked today
	}

	// Hit the GitHub Releases API — at most once per day.
	release, err := selfupdate.LatestRelease()
	if err == nil && release != nil && selfupdate.IsNewer(release.TagName, BuildVersion) {
		fmt.Fprintf(os.Stderr, "A new version of em is available (%s). Run 'em update' to upgrade.\n", release.TagName)
	}

	f, err := os.Create(checkFile)
	if err == nil {
		fmt.Fprintf(f, "%s", today)
		f.Close()
	}
}

func main() {
	secrets.SetDefaultProvider("em")
	checkUpgrade()
	cmd.BuildVersion = BuildVersion
	cmd.BuildGitHash = BuildGitHash
	cmd.BuildDate = BuildDate
	cmd.Execute()
}
