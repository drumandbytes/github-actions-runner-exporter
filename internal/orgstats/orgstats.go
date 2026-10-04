// Package orgstats builds repo/CI, PR, Dependabot and rate-limit stats on a
// slow TTL; none of it needs to be real-time.
package orgstats

import (
	"context"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

// WorkflowCI is one active workflow's latest-run status.
type WorkflowCI struct {
	Name string

	HasRun bool // false if this workflow has never run

	// raw GitHub conclusion; pass/fail is the dashboard's call
	LastConclusion string

	LastRunAt          time.Time
	LastRunDurationSec float64
	LastRunURL         string
}

type RepoStats struct {
	Name       string
	Visibility string // "public" | "private"

	OpenPRs int

	// active workflows only: a disabled one's last run means nothing
	Workflows []WorkflowCI

	// severity -> open count; absent severities have no key
	DependabotAlertsBySeverity map[string]int
}

type Summary struct {
	GeneratedAt time.Time
	Repos       []RepoStats
}

// Client is the subset of *github.Client this package calls; tests fake it.
type Client interface {
	Repos(ctx context.Context) ([]github.Repo, error)
	OpenPRCount(ctx context.Context, repo string) (int, error)
	Workflows(ctx context.Context, repo string) ([]github.Workflow, error)
	DependabotAlerts(ctx context.Context, repo string) ([]github.DependabotAlert, error)
	LatestRunForWorkflow(ctx context.Context, repo string, workflowID int64) (github.WorkflowRun, bool, error)
	RunsCreatedSince(ctx context.Context, repo string, since time.Time) ([]github.WorkflowRun, error)
	RunJobs(ctx context.Context, repo string, runID int64) ([]github.Job, error)
}

func Build(ctx context.Context, client Client, feed *Feed) (Summary, error) {
	repos, err := client.Repos(ctx)
	if err != nil {
		return Summary{}, err
	}

	s := Summary{GeneratedAt: time.Now()}
	for _, r := range repos {
		if r.Archived {
			continue
		}

		stats := RepoStats{Name: r.Name}
		if r.Private {
			stats.Visibility = "private"
		} else {
			stats.Visibility = "public"
		}

		// per-repo errors are swallowed: one repo the token can't see (or with
		// Dependabot off, 404) mustn't blank the rest of the scrape
		if n, err := client.OpenPRCount(ctx, r.Name); err == nil {
			stats.OpenPRs = n
		}

		if workflows, err := client.Workflows(ctx, r.Name); err == nil {
			var active []github.Workflow
			for _, wf := range workflows {
				if wf.State == "active" {
					active = append(active, wf)
				}
			}
			stats.Workflows = feed.Poll(ctx, client, r.Name, active)
		}

		if alerts, err := client.DependabotAlerts(ctx, r.Name); err == nil && len(alerts) > 0 {
			stats.DependabotAlertsBySeverity = map[string]int{}
			for _, a := range alerts {
				stats.DependabotAlertsBySeverity[a.SecurityAdvisory.Severity]++
			}
		}

		s.Repos = append(s.Repos, stats)
	}
	return s, nil
}
