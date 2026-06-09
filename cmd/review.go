package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/spf13/cobra"
)

type reviewPerson struct {
	slug           string
	displayName    string
	jiraEmail      string
	githubUsername string
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Manage people for review reports",
	Long:  `Manage the list of people included in review reports. Each person has a display name, JIRA email, and GitHub username.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.InitConfig(emConfigDir())
	},
	Run: func(cmd *cobra.Command, args []string) {
		people := getReviewPeople()
		if len(people) == 0 {
			fmt.Println("No people configured. Use 'em review add' to add someone.")
		} else {
			fmt.Println("Configured people:")
			for _, p := range people {
				fmt.Printf("  %-25s  jira:%-30s  github:%s\n", p.displayName, p.jiraEmail, p.githubUsername)
			}
		}
		fmt.Println()
		cmd.Help() //nolint:errcheck
	},
}

var (
	reviewAddName           string
	reviewAddJiraEmail      string
	reviewAddGithubUsername string
)

var reviewAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a person to review reports",
	Long: `Add a person to the review people list.

If flags are omitted the command runs interactively, looking up candidates from
configured JIRA and GitHub teams before falling back to manual entry.

Example:
  em review add
  em review add --name "Alice Smith" --jira-email alice@example.com --github alicesmith`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(reviewAddName)
		jiraEmail := strings.TrimSpace(reviewAddJiraEmail)
		githubUsername := strings.TrimSpace(reviewAddGithubUsername)
		if name != "" && jiraEmail != "" && githubUsername != "" {
			return addReviewPerson(name, jiraEmail, githubUsername)
		}
		return interactiveAdd(name, jiraEmail, githubUsername)
	},
}

var reviewRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a person from review reports",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeReviewPerson(args[0])
	},
}

func getReviewConfigAny(key string) any {
	config.InitConfig(emConfigDir()) //nolint:errcheck
	val, _ := config.GetConfigValue(configNamespace, key)
	return val
}

func getReviewPeople() []reviewPerson {
	raw := getReviewConfigAny("people")
	if raw == nil {
		return nil
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	var people []reviewPerson
	for slug, v := range rawMap {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		p := reviewPerson{slug: slug}
		if s, ok := entry["display_name"].(string); ok {
			p.displayName = s
		}
		if s, ok := entry["jira_email"].(string); ok {
			p.jiraEmail = s
		}
		if s, ok := entry["github_username"].(string); ok {
			p.githubUsername = s
		}
		people = append(people, p)
	}
	sort.Slice(people, func(i, j int) bool { return people[i].slug < people[j].slug })
	return people
}

func addReviewPerson(displayName, jiraEmail, githubUsername string) error {
	slug := strings.ToLower(displayName)
	config.SetConfigValue(configNamespace, fmt.Sprintf("people.%s.display_name", slug), displayName)
	config.SetConfigValue(configNamespace, fmt.Sprintf("people.%s.jira_email", slug), jiraEmail)
	config.SetConfigValue(configNamespace, fmt.Sprintf("people.%s.github_username", slug), githubUsername)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Added %s\n", displayName)
	return nil
}

func removeReviewPerson(nameOrSlug string) error {
	slug := strings.ToLower(strings.TrimSpace(nameOrSlug))
	raw := getReviewConfigAny("people")
	if raw == nil {
		return fmt.Errorf("no people configured")
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected people config format")
	}
	if _, exists := rawMap[slug]; !exists {
		return fmt.Errorf("person %q not found", nameOrSlug)
	}
	delete(rawMap, slug)
	config.SetConfigValue(configNamespace, "people", rawMap)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Removed %s\n", nameOrSlug)
	return nil
}

func init() {
	reviewAddCmd.Flags().StringVar(&reviewAddName, "name", "", "Display name")
	reviewAddCmd.Flags().StringVar(&reviewAddJiraEmail, "jira-email", "", "JIRA email address (used in assignee queries)")
	reviewAddCmd.Flags().StringVar(&reviewAddGithubUsername, "github", "", "GitHub username")
	reviewCmd.AddCommand(reviewAddCmd)
	reviewCmd.AddCommand(reviewRemoveCmd)
	reviewCmd.AddCommand(reviewFetchDataCmd)
	rootCmd.AddCommand(reviewCmd)
}
