/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/spf13/cobra"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration for devctl-em",
	Long: `Manage configuration values for devctl-em CLI.

Use this command to get, set, or delete configuration values.
Configuration is stored in ~/.devctl-em/config.yaml

Examples:
  devctl-em config get metrics.endpoint
  devctl-em config set metrics.endpoint https://api.example.com
  devctl-em config delete metrics.endpoint`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use 'devctl-em config --help' for available subcommands")
	},
}

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get [command.key]",
	Short: "Get a configuration value",
	Long: `Get a configuration value from the devctl-em config.

Examples:
  devctl-em config get metrics.endpoint
  devctl-em config get deployment.frequency`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			log.Fatal("Key must be in format 'command.key'")
		}

		command, configKey := parts[0], parts[1]
		if err := config.InitConfig("devctl-em"); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		configData, err := config.FetchConfig(command)
		if err != nil {
			log.Fatalf("Failed to fetch config: %v", err)
		}

		if value, exists := configData[configKey]; exists {
			fmt.Printf("%s = %v\n", key, value)
		} else {
			fmt.Printf("Configuration key '%s' not found\n", key)
		}
	},
}

// setCmd represents the set command
var setCmd = &cobra.Command{
	Use:   "set [command.key] [value]",
	Short: "Set a configuration value",
	Long: `Set a configuration value in the devctl-em config.

Examples:
  devctl-em config set metrics.endpoint https://api.example.com
  devctl-em config set deployment.frequency daily`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			log.Fatal("Key must be in format 'command.key'")
		}

		command, configKey := parts[0], parts[1]
		if err := config.InitConfig("devctl-em"); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		config.SetConfigValue(command, configKey, value)

		fmt.Printf("Set %s = %s\n", key, value)
	},
}

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete [command.key]",
	Short: "Delete a configuration value",
	Long: `Delete a configuration value from the devctl-em config.

Examples:
  devctl-em config delete metrics.endpoint
  devctl-em config delete deployment.frequency`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			log.Fatal("Key must be in format 'command.key'")
		}

		command, configKey := parts[0], parts[1]
		if err := config.InitConfig("devctl-em"); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		config.DeleteConfigValue(command, configKey)
		fmt.Printf("Deleted configuration key '%s'\n", key)
	},
}

func init() {
	// Add subcommands to config
	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(deleteCmd)

	// Add config to root
	rootCmd.AddCommand(configCmd)
}
