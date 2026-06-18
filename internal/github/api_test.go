package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"

	"github.com/danlafeir/em/pkg/httputil"
)

// newTestClient creates a Client backed by a TLS mock httptest.Server.
// go-gh always uses HTTPS, so we use NewTLSServer and trust its certificate.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)

	opts := ghapi.ClientOptions{
		AuthToken: "fake-token",
		Host:      strings.TrimPrefix(ts.URL, "https://"),
		Transport: ts.Client().Transport, // trusts the test server's self-signed cert
	}
	rest, err := ghapi.NewRESTClient(opts)
	if err != nil {
		ts.Close()
		t.Fatalf("failed to create REST client: %v", err)
	}
	return &Client{rest: rest}, ts
}

// jsonHandler returns a handler that always responds with the given JSON string.
func jsonHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	})
}

// -- ListWorkflows ------------------------------------------------------------

func TestListWorkflows(t *testing.T) {
	resp := WorkflowListResponse{
		TotalCount: 2,
		Workflows: []Workflow{
			{ID: 1, Name: "Deploy", Path: ".github/workflows/deploy.yml", State: "active"},
			{ID: 2, Name: "Test", Path: ".github/workflows/test.yml", State: "active"},
		},
	}
	body, _ := json.Marshal(resp)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	workflows, err := client.ListWorkflows(context.Background(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}

	if len(workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(workflows))
	}
	if workflows[0].Name != "Deploy" {
		t.Errorf("expected first workflow 'Deploy', got %q", workflows[0].Name)
	}
	if workflows[1].Name != "Test" {
		t.Errorf("expected second workflow 'Test', got %q", workflows[1].Name)
	}
}

func TestListWorkflows_Empty(t *testing.T) {
	resp := WorkflowListResponse{TotalCount: 0, Workflows: []Workflow{}}
	body, _ := json.Marshal(resp)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	workflows, err := client.ListWorkflows(context.Background(), "org", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(workflows) != 0 {
		t.Errorf("expected 0 workflows, got %d", len(workflows))
	}
}

// -- ListWorkflowRuns ---------------------------------------------------------

func TestListWorkflowRuns(t *testing.T) {
	run1 := WorkflowRun{
		ID:         101,
		Name:       "Deploy",
		Status:     "completed",
		Conclusion: "success",
		HeadBranch: "main",
		CreatedAt:  time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC),
	}
	run2 := WorkflowRun{
		ID:         102,
		Name:       "Deploy",
		Status:     "completed",
		Conclusion: "failure",
		HeadBranch: "main",
		CreatedAt:  time.Date(2024, 6, 5, 10, 0, 0, 0, time.UTC),
	}
	resp := WorkflowRunsResponse{TotalCount: 2, WorkflowRuns: []WorkflowRun{run1, run2}}
	body, _ := json.Marshal(resp)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	from := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	runs, err := client.ListWorkflowRuns(context.Background(), "myorg", "myrepo", 42, "main", from, to)
	if err != nil {
		t.Fatalf("ListWorkflowRuns failed: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].ID != 101 {
		t.Errorf("expected run ID 101, got %d", runs[0].ID)
	}
	if runs[1].Conclusion != "failure" {
		t.Errorf("expected second run conclusion 'failure', got %q", runs[1].Conclusion)
	}
}

// -- ListUserTeams ------------------------------------------------------------

func TestListUserTeams_FiltersByOrg(t *testing.T) {
	teams := []Team{
		{ID: 1, Slug: "platform", Name: "Platform", Organization: TeamOrg{Login: "myorg"}},
		{ID: 2, Slug: "ops", Name: "Ops", Organization: TeamOrg{Login: "otherorg"}},
		{ID: 3, Slug: "backend", Name: "Backend", Organization: TeamOrg{Login: "myorg"}},
	}
	body, _ := json.Marshal(teams)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	result, err := client.ListUserTeams(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("ListUserTeams failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 teams for myorg, got %d", len(result))
	}
	for _, team := range result {
		if strings.ToLower(team.Organization.Login) != "myorg" {
			t.Errorf("unexpected org %q in filtered results", team.Organization.Login)
		}
	}
}

func TestListUserTeams_OrgFilterCaseInsensitive(t *testing.T) {
	teams := []Team{
		{ID: 1, Slug: "team-a", Name: "Team A", Organization: TeamOrg{Login: "MyOrg"}},
	}
	body, _ := json.Marshal(teams)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	result, err := client.ListUserTeams(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 team (case-insensitive match), got %d", len(result))
	}
}

func TestListUserTeams_NoMatchingOrg(t *testing.T) {
	teams := []Team{
		{ID: 1, Slug: "ops", Organization: TeamOrg{Login: "otherorg"}},
	}
	body, _ := json.Marshal(teams)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	result, err := client.ListUserTeams(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 teams, got %d", len(result))
	}
}

// -- ListTeamRepos ------------------------------------------------------------

func TestListTeamRepos(t *testing.T) {
	repos := []Repository{
		{ID: 1, Name: "api", FullName: "myorg/api", Owner: RepositoryOwner{Login: "myorg"}},
		{ID: 2, Name: "frontend", FullName: "myorg/frontend", Owner: RepositoryOwner{Login: "myorg"}},
	}
	body, _ := json.Marshal(repos)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	result, err := client.ListTeamRepos(context.Background(), "myorg", "platform")
	if err != nil {
		t.Fatalf("ListTeamRepos failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(result))
	}
	if result[0].Name != "api" {
		t.Errorf("expected first repo 'api', got %q", result[0].Name)
	}
}

// -- ListTeamMembers ----------------------------------------------------------

func TestListTeamMembers(t *testing.T) {
	// The members endpoint returns logins but no display name.
	members := []User{{Login: "alicesmith"}, {Login: "bob-jones"}}
	body, _ := json.Marshal(members)

	client, ts := newTestClient(t, jsonHandler(string(body)))
	defer ts.Close()

	result, err := client.ListTeamMembers(context.Background(), "myorg", "platform")
	if err != nil {
		t.Fatalf("ListTeamMembers failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 members, got %d", len(result))
	}
	if result[0].Login != "alicesmith" {
		t.Errorf("expected first member 'alicesmith', got %q", result[0].Login)
	}
	if result[0].Name != "" {
		t.Errorf("members endpoint should not carry a name, got %q", result[0].Name)
	}
}

// -- GetUser ------------------------------------------------------------------

func TestGetUser_PopulatesName(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"login":"alicesmith","name":"Alice Smith"}`)
	})
	client, ts := newTestClient(t, handler)
	defer ts.Close()

	u, err := client.GetUser(context.Background(), "alicesmith")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if u.Name != "Alice Smith" {
		t.Errorf("Name = %q, want %q", u.Name, "Alice Smith")
	}
	if u.Login != "alicesmith" {
		t.Errorf("Login = %q, want %q", u.Login, "alicesmith")
	}
	if !strings.HasSuffix(gotPath, "/users/alicesmith") {
		t.Errorf("request path = %q, want suffix /users/alicesmith", gotPath)
	}
}

// -- HTTP error classification -----------------------------------------------

func TestHTTPErrors_ReturnTypedAPIError(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		mustContain string
	}{
		{"401 invalid token", 401, `{"message":"Bad credentials"}`, "em metrics github config"},
		{"403 missing scope", 403, `{"message":"Resource not accessible by personal access token"}`, "`repo` scope"},
		{"404 unknown repo", 404, `{"message":"Not Found"}`, "org, team slug"},
		{"500 server error", 500, `{"message":"Server Error"}`, "transient"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			client, ts := newTestClient(t, h)
			defer ts.Close()

			_, err := client.ListWorkflows(context.Background(), "myorg", "myrepo")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var apiErr *httputil.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *httputil.APIError, got %T: %v", err, err)
			}
			if apiErr.Provider != httputil.ProviderGitHub {
				t.Errorf("Provider = %q, want GitHub", apiErr.Provider)
			}
			if apiErr.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tc.status)
			}
			if !strings.Contains(err.Error(), tc.mustContain) {
				t.Errorf("error message %q does not contain %q", err.Error(), tc.mustContain)
			}
		})
	}
}
