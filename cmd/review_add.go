package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var githubUsernameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// validJiraEmail reports whether s looks like an email address.
func validJiraEmail(s string) bool { return strings.Contains(s, "@") }

// validGithubUsername reports whether s is a syntactically valid GitHub username.
func validGithubUsername(s string) bool { return githubUsernameRe.MatchString(s) }

// validateAddFields rejects malformed non-empty values so that bad input
// supplied via flags is never persisted. Empty values are left for the
// interactive prompts to collect.
func validateAddFields(jiraEmail, githubUsername string) error {
	if jiraEmail != "" && !validJiraEmail(jiraEmail) {
		return fmt.Errorf("invalid jira email %q: must contain @", jiraEmail)
	}
	if githubUsername != "" && !validGithubUsername(githubUsername) {
		return fmt.Errorf("invalid github username %q: must contain only letters, numbers, and hyphens", githubUsername)
	}
	return nil
}

func interactiveAdd(prefillName, prefillJira, prefillGithub string) error {
	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	// Reject malformed prefills up front so the user isn't sent through the
	// whole flow only to fail at save.
	if err := validateAddFields(prefillJira, prefillGithub); err != nil {
		return err
	}

	// --- Name ---
	name, err := promptName(reader, prefillName)
	if err != nil {
		return err
	}

	// --- JIRA email ---
	jiraEmail, err := collectJiraEmail(ctx, reader, name, prefillJira)
	if err != nil {
		return err
	}

	// --- GitHub username ---
	githubUsername, err := collectGithubUsername(ctx, reader, prefillGithub)
	if err != nil {
		return err
	}

	// --- Existing person check ---
	slug := strings.ToLower(name)
	for _, p := range getReviewPeople() {
		if p.slug == slug {
			fmt.Printf("Person %q already exists (jira: %s, github: %s). Update? [y/N]: ", slug, p.jiraEmail, p.githubUsername)
			line, _ := reader.ReadString('\n')
			if !affirmative(line, false) {
				fmt.Println("Cancelled.")
				return nil
			}
			break
		}
	}

	// --- Confirmation ---
	fmt.Printf("\n  Name:   %s\n  JIRA:   %s\n  GitHub: %s\n", name, jiraEmail, githubUsername)
	fmt.Print("Save? [Y/n]: ")
	line, _ := reader.ReadString('\n')
	if !affirmative(line, true) {
		fmt.Println("Cancelled.")
		return nil
	}

	return addReviewPerson(name, jiraEmail, githubUsername)
}

func promptName(reader *bufio.Reader, prefill string) (string, error) {
	if prefill != "" {
		return prefill, nil
	}
	for {
		fmt.Print("Name: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(line)
		if name != "" {
			return name, nil
		}
		fmt.Println("  Name is required.")
	}
}

func collectJiraEmail(ctx context.Context, reader *bufio.Reader, name, prefill string) (string, error) {
	if prefill != "" {
		return prefill, nil
	}

	client, err := newReviewJiraClient()
	if err != nil {
		fmt.Printf("(JIRA not configured, enter manually: %v)\n", err)
		return promptManualJiraEmail(reader)
	}

	fmt.Printf("Looking up JIRA users matching %q...\n", name)
	users, err := client.SearchUsers(ctx, name)
	if err != nil {
		fmt.Printf("(could not look up JIRA users: %v)\n", err)
		return promptManualJiraEmail(reader)
	}
	if len(users) == 0 {
		fmt.Println("(no JIRA users found)")
		return promptManualJiraEmail(reader)
	}

	for i, u := range users {
		fmt.Printf("  %d) %s (%s)\n", i+1, u.DisplayName, u.EmailAddress)
	}
	fmt.Printf("  %d) Enter manually\n", len(users)+1)

	choice, err := pickFromList(reader, "JIRA email", len(users)+1)
	if err != nil {
		return "", err
	}
	if choice == len(users) { // last option = manual
		return promptManualJiraEmail(reader)
	}
	return users[choice].EmailAddress, nil
}

func collectGithubUsername(ctx context.Context, reader *bufio.Reader, prefill string) (string, error) {
	if prefill != "" {
		return prefill, nil
	}

	org := reviewConfigString("github.org")
	type teamPair struct{ display, githubSlug string }
	var pairs []teamPair

	if org != "" {
		teamsMap, _ := getReviewConfigAny("teams").(map[string]any)
		for display, v := range teamsMap {
			entry, _ := v.(map[string]any)
			gh, _ := entry["github"].(map[string]any)
			if s, _ := gh["slug"].(string); s != "" {
				pairs = append(pairs, teamPair{display: display, githubSlug: s})
			}
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].display < pairs[j].display })
	}

	if org == "" || len(pairs) == 0 {
		return promptManualGithubUsername(reader)
	}

	ghClient, err := newReviewGithubClient()
	if err != nil {
		fmt.Printf("(GitHub not configured, enter manually: %v)\n", err)
		return promptManualGithubUsername(reader)
	}

	fmt.Println("Looking up GitHub team members...")
	type candidate struct {
		login string
		name  string
		team  string
	}
	seen := map[string]bool{}
	var candidates []candidate
	for _, p := range pairs {
		members, err := ghClient.ListTeamMembers(ctx, org, p.githubSlug)
		if err != nil {
			fmt.Printf("  (warning: could not list members for %s: %v)\n", p.githubSlug, err)
			continue
		}
		for _, m := range members {
			if seen[m.Login] {
				continue
			}
			seen[m.Login] = true
			name := m.Name
			// The members listing omits display names; fetch the profile to fill it in.
			if u, err := ghClient.GetUser(ctx, m.Login); err == nil {
				name = u.Name
			}
			candidates = append(candidates, candidate{login: m.Login, name: name, team: p.display})
		}
	}

	if len(candidates) == 0 {
		fmt.Println("(no GitHub team members found)")
		return promptManualGithubUsername(reader)
	}

	for i, c := range candidates {
		label := c.login
		if c.name != "" {
			label += " (" + c.name + ")"
		}
		fmt.Printf("  %d) %s  — %s\n", i+1, label, c.team)
	}
	fmt.Printf("  %d) Enter manually\n", len(candidates)+1)

	choice, err := pickFromList(reader, "GitHub username", len(candidates)+1)
	if err != nil {
		return "", err
	}
	if choice == len(candidates) { // last option = manual
		return promptManualGithubUsername(reader)
	}
	return candidates[choice].login, nil
}

// pickFromList prints a prompt and reads a 1-based selection.
// Returns the 0-based index. Default (blank input) selects item 1.
// Re-prompts once on invalid input.
func pickFromList(reader *bufio.Reader, label string, count int) (int, error) {
	for attempt := 0; attempt < 2; attempt++ {
		fmt.Printf("%s [1]: ", label)
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= count {
			return n - 1, nil
		}
		fmt.Printf("  Please enter a number between 1 and %d.\n", count)
	}
	return 0, fmt.Errorf("invalid selection")
}

func promptManualJiraEmail(reader *bufio.Reader) (string, error) {
	for {
		fmt.Print("JIRA email: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		v := strings.TrimSpace(line)
		if validJiraEmail(v) {
			return v, nil
		}
		fmt.Println("  Must be a valid email address.")
	}
}

// affirmative interprets a yes/no reply. Blank input yields defaultYes; "y" and
// "yes" (any case) are true; anything else is false. Accepting "yes" — not only
// a bare "y" — keeps a natural affirmative from being read as a cancel.
func affirmative(reply string, defaultYes bool) bool {
	switch strings.ToLower(strings.TrimSpace(reply)) {
	case "":
		return defaultYes
	case "y", "yes":
		return true
	default:
		return false
	}
}

func promptManualGithubUsername(reader *bufio.Reader) (string, error) {
	for {
		fmt.Print("GitHub username: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		v := strings.TrimSpace(line)
		if validGithubUsername(v) {
			return v, nil
		}
		fmt.Println("  Must be non-empty and contain only letters, numbers, and hyphens.")
	}
}
