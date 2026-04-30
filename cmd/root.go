/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/danlafeir/em/cmd/metrics"
	"github.com/danlafeir/em/internal/debug"
	"github.com/danlafeir/cli-go/pkg/update"
	"github.com/spf13/cobra"
)

// Build metadata, populated by main.go from -ldflags-injected values.
var (
	BuildVersion    string
	BuildGitHash    string
	BuildDate       string
	BuildLatestHash string
)

// updateConfig returns the update configuration for em
var updateConfig = update.Config{
	AppName: "em",
	Repo:    "danlafeir/em",
	BinDir:  "bin",
}

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update em to the latest version",
	Run: func(cmd *cobra.Command, args []string) {
		update.RunUpdateWithConfig(updateConfig, BuildGitHash, cmd)
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


