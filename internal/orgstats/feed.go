package orgstats

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

const (
	// ponytail: a run still in progress after this long (stuck, or waiting on
	// a deployment approval) stops pinning the watermark and is never counted.
	maxWatermarkLag = 24 * time.Hour
	// longer than maxWatermarkLag, so a run is forgotten only once no
	// watermark can list it again
	seenTTL = 48 * time.Hour
)

// 5s..1h: CI jobs on this org's runners take seconds to tens of minutes.
var jobBuckets = []float64{5, 10, 30, 60, 120, 180, 300, 600, 900, 1200, 1800, 3600}

type runKey struct {
	id      int64
	attempt int
}

type repoState struct {
	// runs created at or after this are listed on the next poll
	watermark time.Time
	// latest completed run per workflow ID
	last map[int64]WorkflowCI
}

// Feed polls each repo's runs created since a per-repo watermark and records
// every finished job once into histograms. History lives in Prometheus; the
// feed only remembers enough to not double-count. A restart starts from "now".
type Feed struct {
	mu    sync.Mutex // Fetcher may run two builds at once
	now   func() time.Time
	start time.Time

	repos    map[string]*repoState
	seenRuns map[runKey]time.Time
	seenJobs map[int64]time.Time

	queue *prometheus.HistogramVec
	run   *prometheus.HistogramVec
	runs  *prometheus.CounterVec
}

func NewFeed() *Feed { return newFeed(time.Now) }

func newFeed(now func() time.Time) *Feed {
	jobLabels := []string{"repo", "workflow", "job", "runner", "runner_label", "conclusion"}
	return &Feed{
		now:      now,
		start:    now(),
		repos:    map[string]*repoState{},
		seenRuns: map[runKey]time.Time{},
		seenJobs: map[int64]time.Time{},
		queue: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "github_job_queue_seconds",
			Help:    "Time a job waited for a runner (created to started), recorded once per finished job.",
			Buckets: jobBuckets,
		}, jobLabels),
		run: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "github_job_run_seconds",
			Help:    "Time a job ran on its runner (started to completed), recorded once per finished job.",
			Buckets: jobBuckets,
		}, jobLabels),
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "github_workflow_runs_total",
			Help: "Completed workflow runs, counted once per run attempt.",
		}, []string{"repo", "workflow", "conclusion"}),
	}
}

func (f *Feed) Describe(ch chan<- *prometheus.Desc) {
	f.queue.Describe(ch)
	f.run.Describe(ch)
	f.runs.Describe(ch)
}

func (f *Feed) Collect(ch chan<- prometheus.Metric) {
	f.queue.Collect(ch)
	f.run.Collect(ch)
	f.runs.Collect(ch)
}

// Poll records the repo's newly finished runs and returns the latest completed
// run of each active workflow. Errors are swallowed like the rest of a repo's
// stats: the watermark just doesn't move past what wasn't processed.
func (f *Feed) Poll(ctx context.Context, client Client, repo string, active []github.Workflow) []WorkflowCI {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.now()
	f.prune(now)

	st, ok := f.repos[repo]
	if !ok {
		st = f.bootstrap(ctx, client, repo, active)
		f.repos[repo] = st
	}
	f.advance(ctx, client, repo, st, now)

	out := make([]WorkflowCI, 0, len(active))
	for _, wf := range active {
		ci := st.last[wf.ID]
		ci.Name = wf.Name
		out = append(out, ci)
	}
	return out
}

// bootstrap seeds the last-run snapshot so it's populated before anything new
// finishes. Records nothing: history before startup isn't replayed.
func (f *Feed) bootstrap(ctx context.Context, client Client, repo string, active []github.Workflow) *repoState {
	st := &repoState{watermark: f.start, last: map[int64]WorkflowCI{}}
	for _, wf := range active {
		run, ok, err := client.LatestRunForWorkflow(ctx, repo, wf.ID)
		if err != nil || !ok {
			continue
		}
		if run.Status != "completed" {
			// in flight at startup: reach back so it's recorded when it finishes
			if run.CreatedAt.Before(st.watermark) {
				st.watermark = run.CreatedAt
			}
			continue
		}
		st.last[wf.ID] = workflowCI(run)
	}
	return st
}

func (f *Feed) advance(ctx context.Context, client Client, repo string, st *repoState, now time.Time) {
	runs, err := client.RunsCreatedSince(ctx, repo, st.watermark)
	if err != nil {
		return
	}

	// pinned: oldest run still to be processed; newest: latest run listed
	var pinned, newest time.Time
	pin := func(t time.Time) {
		if pinned.IsZero() || t.Before(pinned) {
			pinned = t
		}
	}
	for _, r := range runs {
		if r.CreatedAt.After(newest) {
			newest = r.CreatedAt
		}
		if r.Status != "completed" {
			pin(r.CreatedAt)
			continue
		}
		key := runKey{r.ID, r.RunAttempt}
		if _, seen := f.seenRuns[key]; seen {
			continue
		}
		// finished before startup: the previous process may have counted it
		if r.UpdatedAt.After(f.start) {
			jobs, err := client.RunJobs(ctx, repo, r.ID)
			if err != nil {
				pin(r.CreatedAt)
				continue
			}
			f.observe(repo, r, jobs, now)
			f.runs.WithLabelValues(repo, r.Name, r.Conclusion).Inc()
		}
		f.seenRuns[key] = now
		if prev, ok := st.last[r.WorkflowID]; !ok || r.UpdatedAt.After(prev.LastRunAt) {
			st.last[r.WorkflowID] = workflowCI(r)
		}
	}

	switch {
	case !pinned.IsZero():
		st.watermark = maxTime(pinned, now.Add(-maxWatermarkLag))
	case !newest.IsZero():
		// inclusive: runs created in the same second are re-listed and deduped
		st.watermark = newest
	}
}

func (f *Feed) observe(repo string, r github.WorkflowRun, jobs []github.Job, now time.Time) {
	for _, j := range jobs {
		// skipped never queued or ran; no runner means cancelled before pickup
		if j.Conclusion == "skipped" || j.RunnerName == "" {
			continue
		}
		// a re-run carries its untouched jobs over under the same ID
		if _, seen := f.seenJobs[j.ID]; seen {
			continue
		}
		f.seenJobs[j.ID] = now

		labels := []string{repo, r.Name, j.Name, runnerName(j), runnerLabel(j.Labels), j.Conclusion}
		if !j.CreatedAt.IsZero() && !j.StartedAt.Before(j.CreatedAt) {
			f.queue.WithLabelValues(labels...).Observe(j.StartedAt.Sub(j.CreatedAt).Seconds())
		}
		if !j.StartedAt.IsZero() && !j.CompletedAt.Before(j.StartedAt) {
			f.run.WithLabelValues(labels...).Observe(j.CompletedAt.Sub(j.StartedAt).Seconds())
		}
	}
}

func (f *Feed) prune(now time.Time) {
	for k, t := range f.seenRuns {
		if now.Sub(t) > seenTTL {
			delete(f.seenRuns, k)
		}
	}
	for k, t := range f.seenJobs {
		if now.Sub(t) > seenTTL {
			delete(f.seenJobs, k)
		}
	}
}

// runnerName collapses GitHub-hosted runners: their names are unique per job.
func runnerName(j github.Job) string {
	if j.RunnerGroupName == "GitHub Actions" {
		return "github-hosted"
	}
	return j.RunnerName
}

// runnerLabel is the job's runs-on labels minus the generic "self-hosted",
// e.g. "oracle-arm64": what tells runner pools apart.
func runnerLabel(labels []string) string {
	l := slices.DeleteFunc(slices.Clone(labels), func(s string) bool { return s == "self-hosted" })
	slices.Sort(l)
	return strings.Join(l, ",")
}

func workflowCI(r github.WorkflowRun) WorkflowCI {
	return WorkflowCI{
		HasRun:             true,
		LastConclusion:     r.Conclusion,
		LastRunAt:          r.UpdatedAt,
		LastRunDurationSec: r.UpdatedAt.Sub(r.CreatedAt).Seconds(),
		LastRunURL:         r.HTMLURL,
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
