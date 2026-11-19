package github

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunsCreatedSincePaginates(t *testing.T) {
	var created string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			created = r.URL.Query().Get("created")
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next", <http://%s%s?page=2>; rel="last"`, r.Host, r.URL.Path, r.Host, r.URL.Path))
			_, _ = w.Write([]byte(`{"workflow_runs":[{"id":1},{"id":2}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"workflow_runs":[{"id":3,"created_at":"2026-10-04T10:00:00Z"}]}`))
	}))
	defer srv.Close()

	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	runs, err := c.RunsCreatedSince(context.Background(), "repo", time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 || runs[2].ID != 3 || runs[2].CreatedAt.Hour() != 10 {
		t.Fatalf("runs = %+v", runs)
	}
	if created != ">=2026-10-04T09:00:00Z" {
		t.Fatalf("created filter = %q", created)
	}
}

func TestRunJobsPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/org/repo/actions/runs/7/jobs" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
			_, _ = w.Write([]byte(`{"jobs":[{"id":1,"started_at":null}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"jobs":[{"id":2,"labels":["self-hosted","oracle-x64"]}]}`))
	}))
	defer srv.Close()

	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	jobs, err := c.RunJobs(context.Background(), "repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || !jobs[0].StartedAt.IsZero() || jobs[1].Labels[1] != "oracle-x64" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestRateLimitFromHeaders(t *testing.T) {
	now := time.Unix(1000, 0)
	// path -> remaining, reset; two windows at once, like GitHub does
	budgets := map[string][2]string{
		"/orgs/org/repos":                   {"3790", "2000"},
		"/repos/org/repo/dependabot/alerts": {"4900", "3000"},
		"/search":                           {"1", "2000"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := budgets[r.URL.Path]
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", b[0])
		w.Header().Set("X-RateLimit-Reset", b[1])
		w.Header().Set("X-RateLimit-Resource", "core")
		if r.URL.Path == "/search" {
			w.Header().Set("X-RateLimit-Resource", "search")
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	c.now = func() time.Time { return now }
	if got := c.RateLimits(); len(got) != 0 {
		t.Fatalf("before any request: %+v", got)
	}
	ctx := context.Background()
	var out []any
	_, _ = c.Repos(ctx)
	_, _ = c.DependabotAlerts(ctx, "repo")
	_ = c.get(ctx, srv.URL+"/search", &out) // another resource's budget
	want := map[time.Time]RateLimit{
		time.Unix(2000, 0).UTC(): {Limit: 5000, Remaining: 3790},
		time.Unix(3000, 0).UTC(): {Limit: 5000, Remaining: 4900},
	}
	if got := c.RateLimits(); !maps.Equal(got, want) {
		t.Fatalf("RateLimits = %+v, want both core windows and not search", got)
	}

	now = time.Unix(2500, 0) // the 3790 window has reset
	if got := c.RateLimits(); len(got) != 1 || got[time.Unix(3000, 0).UTC()].Remaining != 4900 {
		t.Fatalf("after reset: %+v, want only the later window", got)
	}
}

func TestReleasesPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
			_, _ = w.Write([]byte(`[{"tag_name":"v2","assets":[{"name":"a","download_count":5}]}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v1","draft":true}]`))
	}))
	defer srv.Close()

	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	rels, err := c.Releases(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 || rels[0].Assets[0].DownloadCount != 5 || !rels[1].Draft {
		t.Fatalf("releases = %+v", rels)
	}
}

// pagedServer serves body1 on the first request and body2 on page=2, with a Link header between them.
func pagedServer(body1, body2 string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
			_, _ = w.Write([]byte(body1))
			return
		}
		_, _ = w.Write([]byte(body2))
	}))
}

func TestReposPaginates(t *testing.T) {
	srv := pagedServer(`[{"name":"a"}]`, `[{"name":"b","fork":true}]`)
	defer srv.Close()
	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	repos, err := c.Repos(context.Background())
	if err != nil || len(repos) != 2 || repos[1].Name != "b" || !repos[1].Fork {
		t.Fatalf("repos = %+v, %v", repos, err)
	}
}

func TestRunnersPaginates(t *testing.T) {
	srv := pagedServer(`{"total_count":2,"runners":[{"name":"r1"}]}`, `{"total_count":2,"runners":[{"name":"r2"}]}`)
	defer srv.Close()
	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	runners, err := c.Runners(context.Background())
	if err != nil || len(runners) != 2 || runners[1].Name != "r2" {
		t.Fatalf("runners = %+v, %v", runners, err)
	}
}

func TestWorkflowsPaginates(t *testing.T) {
	srv := pagedServer(`{"total_count":2,"workflows":[{"id":1,"name":"ci"}]}`, `{"total_count":2,"workflows":[{"id":2,"name":"deploy"}]}`)
	defer srv.Close()
	c := NewClient("org", "token", time.Second)
	c.baseURL = srv.URL
	wfs, err := c.Workflows(context.Background(), "repo")
	if err != nil || len(wfs) != 2 || wfs[1].ID != 2 {
		t.Fatalf("workflows = %+v, %v", wfs, err)
	}
}
