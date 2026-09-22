// Package collector adapts a summary.Fetcher into a prometheus.Collector,
// so metrics are computed fresh from the shared cache on every scrape
// rather than accumulated/pushed.
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/summary"
)

const namespace = "github"

type Collector struct {
	fetcher *summary.Fetcher

	up         *prometheus.Desc
	runnerUp   *prometheus.Desc
	runnerBusy *prometheus.Desc
}

// New builds the collector. cacheTTL is only used to document the
// caching behavior in each metric's HELP text, per Prometheus's own
// guidance - it doesn't change the actual caching (that's
// summary.Fetcher's job).
func New(fetcher *summary.Fetcher, cacheTTL time.Duration) *Collector {
	cacheNote := fmt.Sprintf(" Cached for up to %s to limit load on the GitHub API.", cacheTTL)
	desc := func(subsystem, name, help string, labels []string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, name), help+cacheNote, labels, nil)
	}

	return &Collector{
		fetcher: fetcher,
		up: desc("", "up",
			"Whether the last scrape of the GitHub API succeeded (1) or a stale cache is being served (0).", nil),
		runnerUp: desc("runner", "up",
			"Whether the self-hosted runner is registered and online (1) or offline (0).", []string{"runner", "os"}),
		runnerBusy: desc("runner", "busy",
			"Whether the runner is currently executing a job (1) or idle (0).", []string{"runner", "os"}),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	s, err := c.fetcher.Get(context.Background())
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)

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
