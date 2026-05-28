/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/danlafeir/em/cmd/metrics"
	"github.com/danlafeir/em/internal/debug"
	"github.com/danlafeir/em/internal/selfupdate"
	"github.com/spf13/cobra"
)

// Build metadata, populated by main.go from -ldflags-injected values.
var (
	BuildVersion string
	BuildGitHash string
	BuildDate    string
)

// updateCmd downloads and installs the latest GitHub Release of em.
// An optional channel argument ("stable" or "beta") switches the update
// channel and persists the choice for future `em update` runs.
var updateCmd = &cobra.Command{
	Use:   "update [channel]",
	Short: "Update em to the latest version",
	Long: `Update em to the latest version.

  em update            – update using the current channel (stable by default)
  em update beta       – switch to the beta channel and update to the latest pre-release
  em update stable     – switch back to the stable channel`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		channel := metrics.GetUpdateChannel()
		if len(args) == 1 {
			switch args[0] {
			case "beta", "stable":
				channel = args[0]
				metrics.SetUpdateChannel(channel)
				cmd.Printf("Switched to %s channel.\n", channel)
			default:
				cmd.PrintErrf("Unknown channel %q — use 'stable' or 'beta'.\n", args[0])
				return
			}
		}
		selfupdate.RunWithChannel(BuildVersion, channel, cmd)
	},
}

// versionCmd prints build metadata. Mirrors `em --version`.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print em version, commit, and build date",
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Println(versionString())
	},
}

func versionString() string {
	return fmt.Sprintf("em %s (commit %s, built %s)", BuildVersion, BuildGitHash, BuildDate)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "em",
	Short: "Engineering manager CLI tools for metrics and reporting",
	Long: `em provides CLI tools for engineering managers to generate metrics reports and insights.`,
}

// debugFlag is set by the persistent --debug flag on the root command.
// Read via DebugEnabled().
var debugFlag bool

// DebugEnabled reports whether --debug was passed.
func DebugEnabled() bool { return debugFlag }

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Build vars are populated by main.go before this is called, so the
	// version template can read them directly.
	rootCmd.Version = versionString()

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

const helpTemplate = `{{with .Long}}{{. | trimRightSpace}}

{{end}}{{if .HasAvailableSubCommands}}Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

{{end}}{{if .HasAvailableLocalFlags}}Flags:
{{.LocalFlags.FlagUsages | trimRightSpace}}

{{end}}{{if .HasAvailableInheritedFlags}}Global Flags:
{{.InheritedFlags.FlagUsages | trimRightSpace}}

{{end}}{{if .HasAvailableSubCommands}}Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false,
		"Print HTTP requests and intermediate calculations to stderr")

	// OnInitialize fires for every cobra Execute call after flag parsing,
	// regardless of which subcommand defines its own PersistentPreRun. This
	// is the safest hook to propagate --debug into the shared debug package.
	cobra.OnInitialize(func() {
		debug.Set(debugFlag)
	})

	rootCmd.AddCommand(metrics.MetricsCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)

	// `em --version` should print just our version string, not Cobra's default
	// "<name> version <version>" wrapper. The Version itself is set in
	// Execute() once main.go has populated the build-metadata vars.
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// Disable default commands
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.SetHelpTemplate(helpTemplate)
}


