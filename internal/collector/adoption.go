package collector

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/adoption"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/fetch"
)

// Adoption exposes the adoption.Summary; registered only when ADOPTION_REPOS is set.
type Adoption struct {
	fetcher *fetch.Fetcher[adoption.Summary]

	up, stars, forks, watchers, openIssues, downloads *prometheus.Desc
	trafficOK                                         *prometheus.Desc
	viewsDaily, viewsUniquesDaily                     *prometheus.Desc
	clonesDaily, clonesUniquesDaily                   *prometheus.Desc
	refViews, refUniques, pathViews, pathUniques      *prometheus.Desc
}

func NewAdoption(f *fetch.Fetcher[adoption.Summary], ttl time.Duration) *Adoption {
	note := fmt.Sprintf(" Polled every %s.", ttl)
	d := func(subsystem, name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, name), help+note, labels, nil)
	}
	const window = " Rolling 14-day window on GitHub's side; the series persist in Prometheus after it rolls off."
	return &Adoption{
		fetcher: f,
		up:      d("adoption", "up", "Whether the last adoption poll succeeded (1) or a stale cache is being served (0)."),

		stars:      d("repo", "stars", "Stargazers.", "repo"),
		forks:      d("repo", "forks", "Forks.", "repo"),
		watchers:   d("repo", "watchers", "Watchers (subscribers).", "repo"),
		openIssues: d("repo", "open_issues", "Open issues, excluding pull requests.", "repo"),
		downloads: d("release", "asset_downloads", "Cumulative download count of a release asset. Chart with increase(), not raw.",
			"repo", "tag", "asset"),

		trafficOK: d("repo", "traffic_ok", "Whether the traffic API answered (1) or the token lacks push access (0).", "repo"),
		viewsDaily: d("repo", "traffic_views_daily", "Page views for one UTC day. Today's date is partial."+window,
			"repo", "date"),
		viewsUniquesDaily: d("repo", "traffic_view_uniques_daily", "Unique visitors for one UTC day."+window, "repo", "date"),
		clonesDaily:       d("repo", "traffic_clones_daily", "Git clones for one UTC day."+window, "repo", "date"),
		clonesUniquesDaily: d("repo", "traffic_clone_uniques_daily", "Unique cloners for one UTC day."+window,
			"repo", "date"),
		refViews:    d("repo", "traffic_referrer_views", "Views from a top-10 referrer."+window, "repo", "referrer"),
		refUniques:  d("repo", "traffic_referrer_uniques", "Unique visitors from a top-10 referrer."+window, "repo", "referrer"),
		pathViews:   d("repo", "traffic_path_views", "Views of a top-10 content path."+window, "repo", "path"),
		pathUniques: d("repo", "traffic_path_uniques", "Unique visitors of a top-10 content path."+window, "repo", "path"),
	}
}

func (a *Adoption) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		a.up, a.stars, a.forks, a.watchers, a.openIssues, a.downloads, a.trafficOK,
		a.viewsDaily, a.viewsUniquesDaily, a.clonesDaily, a.clonesUniquesDaily,
		a.refViews, a.refUniques, a.pathViews, a.pathUniques,
	} {
		ch <- d
	}
}

func (a *Adoption) Collect(ch chan<- prometheus.Metric) {
	s, err := a.fetcher.Get()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(a.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(a.up, prometheus.GaugeValue, 1)

	g := func(d *prometheus.Desc, v float64, l ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, l...)
	}
	for _, r := range s.Repos {
		g(a.stars, float64(r.Stars), r.Name)
		g(a.forks, float64(r.Forks), r.Name)
		g(a.watchers, float64(r.Watchers), r.Name)
		g(a.openIssues, float64(r.OpenIssues), r.Name)
		for _, x := range r.Downloads {
			g(a.downloads, float64(x.Count), r.Name, x.Tag, x.Asset)
		}

		if !r.TrafficOK {
			g(a.trafficOK, 0, r.Name)
			continue
		}
		g(a.trafficOK, 1, r.Name)
		for _, x := range r.Views {
			g(a.viewsDaily, float64(x.Count), r.Name, x.Date)
			g(a.viewsUniquesDaily, float64(x.Uniques), r.Name, x.Date)
		}
		for _, x := range r.Clones {
			g(a.clonesDaily, float64(x.Count), r.Name, x.Date)
			g(a.clonesUniquesDaily, float64(x.Uniques), r.Name, x.Date)
		}
		for _, x := range r.Referrers {
			g(a.refViews, float64(x.Count), r.Name, x.Name)
			g(a.refUniques, float64(x.Uniques), r.Name, x.Name)
		}
		for _, x := range r.Paths {
			g(a.pathViews, float64(x.Count), r.Name, x.Name)
			g(a.pathUniques, float64(x.Uniques), r.Name, x.Name)
		}
	}
}
