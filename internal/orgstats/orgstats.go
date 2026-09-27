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
	RateLimit   github.RateLimit
}

func Build(ctx context.Context, client *github.Client) (Summary, error) {
	repos, err := client.Repos(ctx)
	if err != nil {
		return Summary{}, err
	}

	rateLimit, err := client.RateLimit(ctx)
	if err != nil {
		return Summary{}, err
	}

	s := Summary{GeneratedAt: time.Now(), RateLimit: rateLimit}
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
			for _, wf := range workflows {
				if wf.State != "active" {
					continue
				}
				stats.Workflows = append(stats.Workflows, buildWorkflowCI(ctx, client, r.Name, wf))
			}
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

func buildWorkflowCI(ctx context.Context, client *github.Client, repo string, wf github.Workflow) WorkflowCI {
	ci := WorkflowCI{Name: wf.Name}

	run, ok, err := client.LatestRunForWorkflow(ctx, repo, wf.ID)
	if err != nil || !ok || run.Status != "completed" {
		return ci
	}

	ci.HasRun = true
	ci.LastConclusion = run.Conclusion
	ci.LastRunURL = run.HTMLURL
	created, cErr := time.Parse(time.RFC3339, run.CreatedAt)
	updated, uErr := time.Parse(time.RFC3339, run.UpdatedAt)
	if uErr == nil {
		ci.LastRunAt = updated
	}
	if cErr == nil && uErr == nil {
		ci.LastRunDurationSec = updated.Sub(created).Seconds()
	}
	return ci
}
