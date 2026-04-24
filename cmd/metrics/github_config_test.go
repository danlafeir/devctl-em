package metrics

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/danlafeir/em/internal/testutil/mockgithub"
)

// TestRunGhTeamConfigUpdate_EmptyWorkflows_ChangeRejected verifies that typing
// 'c' when no repos are configured prints a message and loops rather than
// panicking or returning an error.
func TestRunGhTeamConfigUpdate_EmptyWorkflows_ChangeRejected(t *testing.T) {
	silenceStdout(t)
	ds := mockgithub.SmallDataset()
	ts := mockgithub.New(ds).Start()
	defer ts.Close()

	client, err := mockgithub.NewClient(ts)
	if err != nil {
		t.Fatal(err)
	}

	// 'c' should be rejected gracefully (no repos yet), then 'd' exits.
	reader := bufio.NewReader(strings.NewReader("c\nd\n"))
	err = runGhTeamConfigUpdate(context.Background(), reader, client, "acme-org", "platform", "test-team", map[string][]string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestRunGhTeamConfigUpdate_EmptyWorkflows_DoneExits verifies that 'd' exits
// cleanly when called with an empty workflow map (the "choose specific repos" entry point).
func TestRunGhTeamConfigUpdate_EmptyWorkflows_DoneExits(t *testing.T) {
	silenceStdout(t)
	ds := mockgithub.SmallDataset()
	ts := mockgithub.New(ds).Start()
	defer ts.Close()

	client, err := mockgithub.NewClient(ts)
	if err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(strings.NewReader("d\n"))
	err = runGhTeamConfigUpdate(context.Background(), reader, client, "acme-org", "platform", "test-team", map[string][]string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
