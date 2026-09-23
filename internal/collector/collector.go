// Package collector adapts runners.Summary and orgstats.Summary into a
// prometheus.Collector, so metrics are computed fresh from the shared
// caches on every scrape rather than accumulated/pushed.
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/fetch"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/orgstats"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/runners"
)

const namespace = "github"

type Collector struct {
	runnerFetcher *fetch.Fetcher[runners.Summary]
	orgFetcher    *fetch.Fetcher[orgstats.Summary]

	runnerUp     *prometheus.Desc
	runnerBusy   *prometheus.Desc
	runnersUp    *prometheus.Desc
	orgUp        *prometheus.Desc
	reposTotal   *prometheus.Desc
	rateLimit    *prometheus.Desc
	rateLimitCap *prometheus.Desc

	repoOpenPRs     *prometheus.Desc
	repoCISuccess   *prometheus.Desc
	repoCILastRunAt *prometheus.Desc
	repoCIDuration  *prometheus.Desc
	repoDependabot  *prometheus.Desc
}

// New builds the collector. runnerCacheTTL/orgCacheTTL are only used to
// document each metric's caching behavior in its HELP text, per
// Prometheus's own guidance - they don't change the actual caching
// (that's each Fetcher's job).
func New(runnerFetcher *fetch.Fetcher[runners.Summary], orgFetcher *fetch.Fetcher[orgstats.Summary], runnerCacheTTL, orgCacheTTL time.Duration) *Collector {
	runnerNote := fmt.Sprintf(" Cached for up to %s.", runnerCacheTTL)
	orgNote := fmt.Sprintf(" Cached for up to %s - not real-time by design, see internal/orgstats.", orgCacheTTL)
	desc := func(subsystem, name, help string, labels []string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, name), help, labels, nil)
	}

	return &Collector{
		runnerFetcher: runnerFetcher,
		orgFetcher:    orgFetcher,

		runnersUp: desc("runners", "up",
			"Whether the last scrape of the runners API succeeded (1) or a stale cache is being served (0)."+runnerNote, nil),
		runnerUp: desc("runner", "up",
			"Whether the self-hosted runner is registered and online (1) or offline (0)."+runnerNote, []string{"runner", "os"}),
		runnerBusy: desc("runner", "busy",
			"Whether the runner is currently executing a job (1) or idle (0)."+runnerNote, []string{"runner", "os"}),

		orgUp: desc("org", "up",
			"Whether the last scrape of the org/repo stats succeeded (1) or a stale cache is being served (0)."+orgNote, nil),
		reposTotal: desc("org", "repos_total",
			"Number of non-archived repos in the org."+orgNote, []string{"visibility"}),
		rateLimit: desc("rate_limit", "remaining",
			"Remaining core API rate-limit budget."+orgNote, nil),
		rateLimitCap: desc("rate_limit", "limit",
			"Total core API rate-limit budget."+orgNote, nil),

		repoOpenPRs: desc("repo", "open_prs",
			"Number of open pull requests."+orgNote, []string{"repo"}),
		repoCISuccess: desc("repo", "ci_success",
			"Whether this workflow's latest completed run succeeded (1) or not (0). Absent if the workflow has never run."+orgNote, []string{"repo", "workflow", "url"}),
		repoCILastRunAt: desc("repo", "ci_last_run_timestamp_seconds",
			"Unix timestamp of this workflow's latest completed run."+orgNote, []string{"repo", "workflow"}),
		repoCIDuration: desc("repo", "ci_last_run_duration_seconds",
			"Duration of this workflow's latest completed run."+orgNote, []string{"repo", "workflow"}),
		repoDependabot: desc("repo", "dependabot_alerts_open",
			"Open Dependabot alerts by severity. Absent for a severity with zero open alerts."+orgNote, []string{"repo", "severity"}),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.collectRunners(ch)
	c.collectOrgStats(ch)
}

func (c *Collector) collectRunners(ch chan<- prometheus.Metric) {
	s, err := c.runnerFetcher.Get(context.Background())
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.runnersUp, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.runnersUp, prometheus.GaugeValue, 1)

	for _, r := range s.Runners {
		up, busy := 0.0, 0.0
		if r.Up {
			up = 1
		}
		if r.Busy {
			busy = 1
		}
		ch <- prometheus.MustNewConstMetric(c.runnerUp, prometheus.GaugeValue, up, r.Name, r.OS)
		ch <- prometheus.MustNewConstMetric(c.runnerBusy, prometheus.GaugeValue, busy, r.Name, r.OS)
	}
}

func (c *Collector) collectOrgStats(ch chan<- prometheus.Metric) {
	s, err := c.orgFetcher.Get(context.Background())
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.orgUp, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.orgUp, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.rateLimit, prometheus.GaugeValue, float64(s.RateLimit.Remaining))
	ch <- prometheus.MustNewConstMetric(c.rateLimitCap, prometheus.GaugeValue, float64(s.RateLimit.Limit))

	visibilityCounts := map[string]int{}
	for _, r := range s.Repos {
		visibilityCounts[r.Visibility]++

		ch <- prometheus.MustNewConstMetric(c.repoOpenPRs, prometheus.GaugeValue, float64(r.OpenPRs), r.Name)

		for _, wf := range r.Workflows {
			if !wf.HasRun {
				continue
			}
			success := 0.0
			if wf.LastSuccess {
				success = 1
			}
			ch <- prometheus.MustNewConstMetric(c.repoCISuccess, prometheus.GaugeValue, success, r.Name, wf.Name, wf.LastRunURL)
			ch <- prometheus.MustNewConstMetric(c.repoCILastRunAt, prometheus.GaugeValue, float64(wf.LastRunAt.Unix()), r.Name, wf.Name)
			ch <- prometheus.MustNewConstMetric(c.repoCIDuration, prometheus.GaugeValue, wf.LastRunDurationSec, r.Name, wf.Name)
		}

		for severity, count := range r.DependabotAlertsBySeverity {
			ch <- prometheus.MustNewConstMetric(c.repoDependabot, prometheus.GaugeValue, float64(count), r.Name, severity)
		}
	}
	for visibility, count := range visibilityCounts {
		ch <- prometheus.MustNewConstMetric(c.reposTotal, prometheus.GaugeValue, float64(count), visibility)
	}
}
