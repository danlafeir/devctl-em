package cmd

import (
	"strings"
	"testing"
)

func TestValidateAddFields(t *testing.T) {
	cases := []struct {
		name           string
		jiraEmail      string
		githubUsername string
		wantErr        string // substring the error must contain, "" means no error
	}{
		{"both valid", "alice@example.com", "alicesmith", ""},
		{"both empty are deferred to prompts", "", "", ""},
		{"empty email skipped", "", "alicesmith", ""},
		{"empty github skipped", "alice@example.com", "", ""},
		{"bad email rejected", "not-an-email", "alicesmith", "jira email"},
		{"bad github space rejected", "alice@example.com", "bad name", "github username"},
		{"bad github punctuation rejected", "alice@example.com", "bad!", "github username"},
		{"github leading hyphen rejected", "alice@example.com", "-nope", "github username"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAddFields(c.jiraEmail, c.githubUsername)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateAddFields(%q, %q) = %v, want nil", c.jiraEmail, c.githubUsername, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validateAddFields(%q, %q) = %v, want error containing %q", c.jiraEmail, c.githubUsername, err, c.wantErr)
			}
		})
	}
}

func TestAffirmative(t *testing.T) {
	cases := []struct {
		reply      string
		defaultYes bool
		want       bool
	}{
		{"", true, true},      // blank takes the default
		{"", false, false},    // blank takes the default
		{"y", false, true},    // explicit yes overrides default-no
		{"Y", false, true},    // case-insensitive
		{"yes", false, true},  // "yes" must not be read as cancel
		{"YES", false, true},  // case-insensitive
		{" yes ", false, true}, // surrounding whitespace trimmed
		{"n", true, false},    // explicit no overrides default-yes
		{"no", true, false},   // "no" is not affirmative
		{"nope", true, false}, // anything unrecognized is not affirmative
	}
	for _, c := range cases {
		if got := affirmative(c.reply, c.defaultYes); got != c.want {
			t.Errorf("affirmative(%q, %v) = %v, want %v", c.reply, c.defaultYes, got, c.want)
		}
	}
}
