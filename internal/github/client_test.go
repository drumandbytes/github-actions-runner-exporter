package github

import (
	"context"
	"fmt"
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
