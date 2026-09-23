// Package orgstats builds repo/CI health, PR counts, Dependabot alert
// counts and rate-limit status for the org. Fetched on a slow cache TTL
// (see internal/fetch and main.go) - unlike runner status, none of this
// needs to be near-real-time, and polling ~3 endpoints per repo across
// a few dozen repos on a short TTL would burn through GitHub's rate
// limit for no benefit.
package orgstats

import (
	"context"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

// WorkflowCI is one active workflow's latest-run status - tracked per
// workflow, not per repo, so a failing workflow can't hide behind a
// later, unrelated, successful one in the same repo.
type WorkflowCI struct {
	Name               string
	HasRun             bool // false if this workflow has never run
	LastSuccess        bool
	LastRunAt          time.Time
	LastRunDurationSec float64
	LastRunURL         string
}

type RepoStats struct {
	Name       string
	Visibility string // "public" | "private"

	OpenPRs int

	// Only active workflows (state == "active") are included - a
	// disabled workflow's stale last run isn't a meaningful signal.
	Workflows []WorkflowCI

	// severity -> open alert count. Only severities that actually occur
	// are present - a repo with zero open "critical" alerts simply has
	// no "critical" key, same convention as pve-metrics-exporter's
	// optional per-sensor critical-threshold series.
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

		// Per-repo call failures below are swallowed deliberately (not
		// propagated as a Build error): a repo the token can't see, or
		// one with Dependabot disabled (404), shouldn't blank out every
		// other repo's data for this scrape.
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
	ci.LastSuccess = run.Conclusion == "success"
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
