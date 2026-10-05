package orgstats

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

var t0 = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

type fakeClient struct {
	latest   map[int64]github.WorkflowRun
	runs     []github.WorkflowRun
	jobs     map[int64][]github.Job
	jobsErr  error
	reposErr error

	sinces   []time.Time
	jobCalls int
}

func (c *fakeClient) Repos(context.Context) ([]github.Repo, error) {
	return []github.Repo{{Name: "repo"}, {Name: "old", Archived: true}}, c.reposErr
}
func (c *fakeClient) OpenPRCount(context.Context, string) (int, error) {
	return 0, errors.New("forbidden")
}
func (c *fakeClient) Workflows(context.Context, string) ([]github.Workflow, error) {
	return []github.Workflow{{ID: 10, Name: "CI", State: "active"}, {ID: 11, Name: "Old", State: "disabled_manually"}}, nil
}
func (c *fakeClient) DependabotAlerts(context.Context, string) ([]github.DependabotAlert, error) {
	return nil, errors.New("disabled")
}
func (c *fakeClient) LatestRunForWorkflow(_ context.Context, _ string, id int64) (github.WorkflowRun, bool, error) {
	r, ok := c.latest[id]
	return r, ok, nil
}
func (c *fakeClient) RunsCreatedSince(_ context.Context, _ string, since time.Time) ([]github.WorkflowRun, error) {
	c.sinces = append(c.sinces, since)
	var out []github.WorkflowRun
	for _, r := range c.runs {
		if !r.CreatedAt.Before(since) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (c *fakeClient) RunJobs(_ context.Context, _ string, id int64) ([]github.Job, error) {
	c.jobCalls++
	return c.jobs[id], c.jobsErr
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func run(id int64, attempt int, status string, created time.Time) github.WorkflowRun {
	return github.WorkflowRun{ID: id, WorkflowID: 10, Name: "CI", RunAttempt: attempt, Status: status,
		Conclusion: map[bool]string{true: "success"}[status == "completed"], CreatedAt: created, UpdatedAt: created.Add(5 * time.Minute)}
}

// job queued 2 min, ran 3 min
func job(id int64, created time.Time) github.Job {
	return github.Job{ID: id, Name: "build", Conclusion: "success", RunnerName: "gha-arm-1",
		Labels: []string{"self-hosted", "oracle-arm64"}, CreatedAt: created,
		StartedAt: created.Add(2 * time.Minute), CompletedAt: created.Add(5 * time.Minute)}
}

var ciActive = []github.Workflow{{ID: 10, Name: "CI", State: "active"}}

func TestBootstrapSeedsLastRunWithoutObserving(t *testing.T) {
	c := &fakeClient{latest: map[int64]github.WorkflowRun{10: run(1, 1, "completed", t0.Add(-time.Hour))}}
	c.runs = []github.WorkflowRun{c.latest[10]}
	f := newFeed((&clock{t0}).now)

	got := f.Poll(context.Background(), c, "repo", ciActive)
	if len(got) != 1 || !got[0].HasRun || got[0].Name != "CI" || got[0].LastRunDurationSec != 300 {
		t.Fatalf("last runs = %+v", got)
	}
	if !c.sinces[0].Equal(t0) {
		t.Fatalf("first watermark = %v, want start %v (no replay)", c.sinces[0], t0)
	}
	if n := testutil.CollectAndCount(f); n != 0 || c.jobCalls != 0 {
		t.Fatalf("bootstrap recorded %d series, %d jobs calls", n, c.jobCalls)
	}
}

func TestBootstrapReachesBackForInFlightRun(t *testing.T) {
	inflight := run(1, 1, "in_progress", t0.Add(-10*time.Minute))
	c := &fakeClient{latest: map[int64]github.WorkflowRun{10: inflight}, runs: []github.WorkflowRun{inflight},
		jobs: map[int64][]github.Job{1: {job(100, t0.Add(-10*time.Minute))}}}
	clk := &clock{t0}
	f := newFeed(clk.now)

	if got := f.Poll(context.Background(), c, "repo", ciActive); got[0].HasRun {
		t.Fatalf("in-flight run reported as last run: %+v", got)
	}
	c.runs[0].Status, c.runs[0].Conclusion, c.runs[0].UpdatedAt = "completed", "success", t0.Add(time.Minute)
	clk.t = t0.Add(5 * time.Minute)
	if got := f.Poll(context.Background(), c, "repo", ciActive); !got[0].HasRun {
		t.Fatal("completed run not picked up")
	}
	if n := testutil.ToFloat64(f.runs.WithLabelValues("repo", "CI", "success")); n != 1 {
		t.Fatalf("runs_total = %v", n)
	}
}

func TestWatermark(t *testing.T) {
	clk := &clock{t0}
	c := &fakeClient{jobs: map[int64][]github.Job{}}
	f := newFeed(clk.now)
	poll := func() time.Time {
		f.Poll(context.Background(), c, "repo", ciActive)
		return c.sinces[len(c.sinces)-1]
	}

	poll()
	slow := run(1, 1, "in_progress", t0.Add(time.Minute))
	fast := run(2, 1, "completed", t0.Add(2*time.Minute))
	c.runs = []github.WorkflowRun{slow, fast}
	clk.t = t0.Add(10 * time.Minute)
	poll()
	if w := poll(); !w.Equal(slow.CreatedAt) {
		t.Fatalf("watermark = %v, want pinned at in-progress run %v", w, slow.CreatedAt)
	}

	c.runs[0].Status, c.runs[0].Conclusion = "completed", "success"
	c.runs[0].UpdatedAt = t0.Add(20 * time.Minute)
	poll()
	if w := poll(); !w.Equal(fast.CreatedAt) {
		t.Fatalf("watermark = %v, want newest run %v", w, fast.CreatedAt)
	}
	if n := testutil.ToFloat64(f.runs.WithLabelValues("repo", "CI", "success")); n != 2 {
		t.Fatalf("runs_total = %v, want both runs once", n)
	}

	// a stuck run pins at most maxWatermarkLag back
	c.runs = append(c.runs, run(3, 1, "waiting", t0.Add(3*time.Minute)))
	clk.t = t0.Add(48 * time.Hour)
	poll()
	if w := poll(); !w.Equal(clk.t.Add(-maxWatermarkLag)) {
		t.Fatalf("watermark = %v, want clamped to %v", w, clk.t.Add(-maxWatermarkLag))
	}
}

func TestJobsErrorIsRetried(t *testing.T) {
	clk := &clock{t0}
	r := run(1, 1, "completed", t0.Add(time.Minute))
	c := &fakeClient{runs: []github.WorkflowRun{r}, jobs: map[int64][]github.Job{1: {job(100, r.CreatedAt)}}, jobsErr: errors.New("502")}
	f := newFeed(clk.now)

	f.Poll(context.Background(), c, "repo", ciActive)
	c.jobsErr = nil
	f.Poll(context.Background(), c, "repo", ciActive)
	if !c.sinces[1].Equal(r.CreatedAt) || c.jobCalls != 2 {
		t.Fatalf("watermark %v, %d jobs calls: failed run not retried", c.sinces[1], c.jobCalls)
	}
	if n := testutil.CollectAndCount(f, "github_job_run_seconds"); n != 1 {
		t.Fatalf("run series = %d", n)
	}
}

func TestDedupe(t *testing.T) {
	clk := &clock{t0}
	r := run(1, 1, "completed", t0.Add(time.Minute))
	c := &fakeClient{runs: []github.WorkflowRun{r}, jobs: map[int64][]github.Job{1: {job(100, r.CreatedAt), job(101, r.CreatedAt)}}}
	f := newFeed(clk.now)

	for range 3 { // overlapping polls re-list the same run
		f.Poll(context.Background(), c, "repo", ciActive)
	}
	if c.jobCalls != 1 {
		t.Fatalf("jobs calls = %d, want 1", c.jobCalls)
	}

	// re-run failed jobs: attempt 2 carries job 100 over, re-runs 101 as 102
	c.runs[0].RunAttempt = 2
	c.runs[0].UpdatedAt = t0.Add(time.Hour)
	c.jobs[1] = []github.Job{job(100, r.CreatedAt), job(102, r.CreatedAt)}
	f.Poll(context.Background(), c, "repo", ciActive)

	if n := testutil.ToFloat64(f.runs.WithLabelValues("repo", "CI", "success")); n != 2 {
		t.Fatalf("runs_total = %v, want one per attempt", n)
	}
	want := `
# HELP github_job_run_seconds Time a job ran on its runner (started to completed), recorded once per finished job.
# TYPE github_job_run_seconds histogram
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="5"} 0
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="10"} 0
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="30"} 0
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="60"} 0
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="120"} 0
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="180"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="300"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="600"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="900"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="1200"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="1800"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="3600"} 3
github_job_run_seconds_bucket{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI",le="+Inf"} 3
github_job_run_seconds_sum{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI"} 540
github_job_run_seconds_count{conclusion="success",job_name="build",repo="repo",runner="gha-arm-1",runner_label="oracle-arm64",workflow="CI"} 3
`
	if err := testutil.CollectAndCompare(f, strings.NewReader(want), "github_job_run_seconds"); err != nil {
		t.Fatal(err)
	}
}

func TestObserveLabels(t *testing.T) {
	clk := &clock{t0}
	r := run(1, 1, "completed", t0.Add(time.Minute))
	hosted := job(1, r.CreatedAt)
	hosted.RunnerName, hosted.RunnerGroupName, hosted.Labels = "GitHub Actions 1000123", "GitHub Actions", []string{"ubuntu-latest"}
	skipped := job(2, r.CreatedAt)
	skipped.Conclusion = "skipped"
	unassigned := job(3, r.CreatedAt)
	unassigned.Conclusion, unassigned.RunnerName = "cancelled", ""
	c := &fakeClient{runs: []github.WorkflowRun{r}, jobs: map[int64][]github.Job{1: {hosted, skipped, unassigned}}}
	f := newFeed(clk.now)

	f.Poll(context.Background(), c, "repo", ciActive)
	if n := testutil.CollectAndCount(f, "github_job_queue_seconds"); n != 1 {
		t.Fatalf("queue series = %d, want only the hosted job", n)
	}
	// creating a missing label set would add a second series
	f.queue.WithLabelValues("repo", "CI", "build", "github-hosted", "ubuntu-latest", "success")
	if n := testutil.CollectAndCount(f, "github_job_queue_seconds"); n != 1 {
		t.Fatal("hosted job not recorded as runner=github-hosted")
	}
}

func TestBuild(t *testing.T) {
	c := &fakeClient{latest: map[int64]github.WorkflowRun{10: run(1, 1, "completed", t0.Add(-time.Hour))}}
	s, err := Build(context.Background(), c, newFeed((&clock{t0}).now))
	if err != nil {
		t.Fatal(err)
	}
	// archived repo dropped, disabled workflow dropped, per-repo errors swallowed
	if len(s.Repos) != 1 || len(s.Repos[0].Workflows) != 1 || !s.Repos[0].Workflows[0].HasRun {
		t.Fatalf("summary = %+v", s)
	}

	c.reposErr = errors.New("401")
	if _, err := Build(context.Background(), c, NewFeed()); err == nil {
		t.Fatal("Repos error not propagated")
	}
}

func TestWorkflowLabelFromWorkflowList(t *testing.T) {
	r := run(1, 1, "completed", t0.Add(time.Minute))
	r.Name = "npm_and_yarn in / - Update #123" // Dependabot's per-run title
	c := &fakeClient{runs: []github.WorkflowRun{r}, jobs: map[int64][]github.Job{1: {job(100, r.CreatedAt)}}}
	f := newFeed((&clock{t0}).now)

	f.Poll(context.Background(), c, "repo", []github.Workflow{{ID: 10, Name: "Dependabot Updates", State: "active"}})
	if n := testutil.ToFloat64(f.runs.WithLabelValues("repo", "Dependabot Updates", "success")); n != 1 {
		t.Fatalf("runs_total{workflow=\"Dependabot Updates\"} = %v, want 1", n)
	}
	if n := testutil.CollectAndCount(f, "github_workflow_runs_total"); n != 1 {
		t.Fatalf("%d runs_total series, want only the workflow-named one", n)
	}
}
