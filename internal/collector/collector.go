// Package collector computes metrics from the shared caches on every scrape.
package collector

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/fetch"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/orgstats"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/runners"
)

const namespace = "github"

type Collector struct {
	org           string
	runnerFetcher *fetch.Fetcher[runners.Summary]
	orgFetcher    *fetch.Fetcher[orgstats.Summary]
	rateLimitFn   func() map[time.Time]github.RateLimit

	runnerUp     *prometheus.Desc
	runnerBusy   *prometheus.Desc
	runnersUp    *prometheus.Desc
	orgInfo      *prometheus.Desc
	orgUp        *prometheus.Desc
	reposTotal   *prometheus.Desc
	rateLimit    *prometheus.Desc
	rateLimitCap *prometheus.Desc
	rateWindow   *prometheus.Desc

	repoOpenPRs             *prometheus.Desc
	repoCILastRunConclusion *prometheus.Desc
	repoCILastRunAt         *prometheus.Desc
	repoCIDuration          *prometheus.Desc
	repoDependabot          *prometheus.Desc
}

// New builds the collector. The intervals only feed HELP text; polling is the Fetchers' job.
func New(org string, runnerFetcher *fetch.Fetcher[runners.Summary], orgFetcher *fetch.Fetcher[orgstats.Summary], rateLimits func() map[time.Time]github.RateLimit, runnerCacheTTL, orgCacheTTL time.Duration) *Collector {
	runnerNote := fmt.Sprintf(" Polled every %s.", runnerCacheTTL)
	orgNote := fmt.Sprintf(" Polled every %s - not real-time by design, see internal/orgstats.", orgCacheTTL)
	desc := func(subsystem, name, help string, labels []string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, name), help, labels, nil)
	}

	return &Collector{
		org:           org,
		runnerFetcher: runnerFetcher,
		orgFetcher:    orgFetcher,
		rateLimitFn:   rateLimits,

		runnersUp: desc("runners", "up",
			"Whether the last scrape of the runners API succeeded (1) or a stale cache is being served (0)."+runnerNote, nil),
		runnerUp: desc("runner", "up",
			"Whether the self-hosted runner is registered and online (1) or offline (0)."+runnerNote, []string{"runner", "os"}),
		runnerBusy: desc("runner", "busy",
			"Whether the runner is currently executing a job (1) or idle (0)."+runnerNote, []string{"runner", "os"}),

		orgUp: desc("org", "up",
			"Whether the last scrape of the org/repo stats succeeded (1) or a stale cache is being served (0)."+orgNote, nil),
		// lets dashboards build github.com links without a hand-set variable
		orgInfo: desc("org", "info",
			"Always 1; the GitHub org this exporter watches is in the org label.", []string{"org"}),
		reposTotal: desc("org", "repos_total",
			"Number of non-archived repos in the org."+orgNote, []string{"visibility"}),
		rateLimit: desc("rate_limit", "remaining",
			"Remaining core API rate-limit budget, as of the latest GitHub response.", nil),
		rateLimitCap: desc("rate_limit", "limit",
			"Total core API rate-limit budget.", nil),
		rateWindow: desc("rate_limit", "window_remaining",
			"Remaining budget per core rate-limit window. GitHub counts per region, so a token can have several windows at once; github_rate_limit_remaining is the lowest.", []string{"reset"}),

		repoOpenPRs: desc("repo", "open_prs",
			"Number of open pull requests."+orgNote, []string{"repo"}),
		// info metric, always 1. conclusion is GitHub's raw string: what counts as
		// broken is the dashboard's call. Absent if the workflow never ran.
		repoCILastRunConclusion: desc("repo", "ci_last_run_conclusion",
			"Always 1; the workflow's latest completed run outcome is in the conclusion label."+orgNote, []string{"repo", "workflow", "url", "conclusion"}),
		repoCILastRunAt: desc("repo", "ci_last_run_timestamp_seconds",
			"Unix timestamp of this workflow's latest completed run."+orgNote, []string{"repo", "workflow"}),
		repoCIDuration: desc("repo", "ci_last_run_duration_seconds",
			"Duration of this workflow's latest completed run, created to last update, so queue time included; see github_job_run_seconds for run time alone."+orgNote, []string{"repo", "workflow"}),
		repoDependabot: desc("repo", "dependabot_alerts_open",
			"Open Dependabot alerts by severity. Absent for a severity with zero open alerts."+orgNote, []string{"repo", "severity"}),
	}
}

// Describe lists every Desc rather than DescribeByCollect: collecting at
// registration would wait for the first GitHub poll, and a failed first poll
// would leave most metrics undescribed.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.runnersUp, c.runnerUp, c.runnerBusy,
		c.orgInfo, c.orgUp, c.reposTotal, c.rateLimit, c.rateLimitCap, c.rateWindow,
		c.repoOpenPRs, c.repoCILastRunConclusion, c.repoCILastRunAt, c.repoCIDuration, c.repoDependabot,
	} {
		ch <- d
	}
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.collectRunners(ch)
	c.collectOrgStats(ch)
	c.collectRateLimits(ch)
	ch <- prometheus.MustNewConstMetric(c.orgInfo, prometheus.GaugeValue, 1, c.org)
}

// collectRateLimits reports every active core window plus the scarcest one,
// which is what runs out first. Absent until the first GitHub response.
func (c *Collector) collectRateLimits(ch chan<- prometheus.Metric) {
	var low github.RateLimit
	for reset, rl := range c.rateLimitFn() {
		ch <- prometheus.MustNewConstMetric(c.rateWindow, prometheus.GaugeValue, float64(rl.Remaining), reset.Format(time.RFC3339))
		if low.Limit == 0 || rl.Remaining < low.Remaining {
			low = rl
		}
	}
	if low.Limit > 0 {
		ch <- prometheus.MustNewConstMetric(c.rateLimit, prometheus.GaugeValue, float64(low.Remaining))
		ch <- prometheus.MustNewConstMetric(c.rateLimitCap, prometheus.GaugeValue, float64(low.Limit))
	}
}

func (c *Collector) collectRunners(ch chan<- prometheus.Metric) {
	s, err := c.runnerFetcher.Get()
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
	s, err := c.orgFetcher.Get()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.orgUp, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.orgUp, prometheus.GaugeValue, 1)

	visibilityCounts := map[string]int{}
	for _, r := range s.Repos {
		visibilityCounts[r.Visibility]++

		ch <- prometheus.MustNewConstMetric(c.repoOpenPRs, prometheus.GaugeValue, float64(r.OpenPRs), r.Name)

		for _, wf := range r.Workflows {
			if !wf.HasRun {
				continue
			}
			ch <- prometheus.MustNewConstMetric(c.repoCILastRunConclusion, prometheus.GaugeValue, 1, r.Name, wf.Name, wf.LastRunURL, wf.LastConclusion)
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
