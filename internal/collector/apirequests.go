package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// APIRequests exposes how many GitHub API requests were full responses (they count against the
// rate limit) and how many were 304s answered from the conditional-request cache (they do not).
type APIRequests struct {
	counts func() (fresh, notModified int64)
	desc   *prometheus.Desc
}

func NewAPIRequests(counts func() (fresh, notModified int64)) *APIRequests {
	return &APIRequests{
		counts: counts,
		desc: prometheus.NewDesc(prometheus.BuildFQName(namespace, "api", "requests_total"),
			"GitHub API requests by result: fresh (a 200, counted against the rate limit) or not_modified (a 304 answered from the cache, not counted).",
			[]string{"result"}, nil),
	}
}

func (a *APIRequests) Describe(ch chan<- *prometheus.Desc) { ch <- a.desc }

func (a *APIRequests) Collect(ch chan<- prometheus.Metric) {
	fresh, notModified := a.counts()
	ch <- prometheus.MustNewConstMetric(a.desc, prometheus.CounterValue, float64(fresh), "fresh")
	ch <- prometheus.MustNewConstMetric(a.desc, prometheus.CounterValue, float64(notModified), "not_modified")
}
