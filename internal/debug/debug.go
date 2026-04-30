// Package debug provides a process-wide flag and Printf-style helper for
// the global `--debug` flag. cmd/root.go writes the flag value here on
// startup; everywhere else just reads via Enabled() / Printf().
package debug

import (
	"fmt"
	"os"
)

var enabled bool

// Set toggles debug output. Call from the cobra root command's PreRun (or
// during flag parsing) so downstream packages see the right value.
func Set(v bool) { enabled = v }

// Enabled reports whether debug output is on.
func Enabled() bool { return enabled }

// Printf writes a single line to stderr, prefixed with [debug], when --debug
// is on. No-op otherwise.
func Printf(format string, args ...any) {
	if !enabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", args...)
}
