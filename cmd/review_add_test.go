package cmd

import "testing"

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
