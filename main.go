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
	"github.com/danlafeir/cli-go/pkg/secrets"
)

// Build metadata is set at build time via -ldflags. See Makefile.
var (
	BuildVersion    = "dev"
	BuildGitHash    = "dev"
	BuildDate       = "unknown"
	BuildLatestHash = "dev"
)

func checkUpgrade() {
	home, err := os.UserHomeDir()
	if err != nil {
		return // fail silently
	}
	checkFile := filepath.Join(home, ".em", "upgrade-check")
	os.MkdirAll(filepath.Dir(checkFile), 0o755)

	today := time.Now().Format("2006-01-02")
	var lastDate, lastHash string
	if f, err := os.Open(checkFile); err == nil {
		fmt.Fscanf(f, "%s %s", &lastDate, &lastHash)
		f.Close()
	}
	if lastDate == today {
		return // already checked today
	}

	// Check remote for latest hash
	remoteHash := BuildLatestHash
	if remoteHash != "" && remoteHash != BuildGitHash {
		fmt.Fprintf(os.Stderr, "A new version of em is available (commit %s). Run 'em update' to upgrade.\n", remoteHash)
	}

	// Write today's check
	f, err := os.Create(checkFile)
	if err == nil {
		fmt.Fprintf(f, "%s %s", today, BuildGitHash)
		f.Close()
	}
}

func main() {
	secrets.SetDefaultProvider("em")
	checkUpgrade()
	cmd.BuildVersion = BuildVersion
	cmd.BuildGitHash = BuildGitHash
	cmd.BuildDate = BuildDate
	cmd.BuildLatestHash = BuildLatestHash
	cmd.Execute()
}
