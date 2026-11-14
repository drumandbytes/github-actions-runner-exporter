// Package adoption gathers public adoption stats (stars, release downloads)
// and repo traffic for a short list of repos, on a slow TTL. Traffic is a
// rolling 14-day window on GitHub's side, so Prometheus keeps the history.
package adoption

import (
	"context"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

type AssetDownloads struct {
	Tag   string
	Asset string
	Count int64
}

type Day struct {
	Date    string // YYYY-MM-DD, UTC
	Count   int
	Uniques int
}

type Hits struct {
	Name    string // referrer host or content path
	Count   int
	Uniques int
}

type RepoStats struct {
	Name string

	Stars, Forks, Watchers, OpenIssues int

	Downloads []AssetDownloads

	// Traffic* are empty and TrafficOK is false when the token lacks push access.
	TrafficOK bool
	Views     []Day
	Clones    []Day
	Referrers []Hits
	Paths     []Hits
}

type Summary struct {
	GeneratedAt time.Time
	Repos       []RepoStats
}

// Client is the subset of *github.Client this package calls; tests fake it.
type Client interface {
	RepoInfo(ctx context.Context, repo string) (github.RepoInfo, error)
	OpenPRCount(ctx context.Context, repo string) (int, error)
	Releases(ctx context.Context, repo string) ([]github.Release, error)
	TrafficViews(ctx context.Context, repo string) ([]github.TrafficDay, error)
	TrafficClones(ctx context.Context, repo string) ([]github.TrafficDay, error)
	TrafficReferrers(ctx context.Context, repo string) ([]github.Referrer, error)
	TrafficPaths(ctx context.Context, repo string) ([]github.PopularPath, error)
}

// Build fails only when a repo's basic info or releases can't be read; a
// missing traffic permission just leaves TrafficOK false.
func Build(ctx context.Context, client Client, repos []string) (Summary, error) {
	s := Summary{GeneratedAt: time.Now()}
	for _, name := range repos {
		info, err := client.RepoInfo(ctx, name)
		if err != nil {
			return Summary{}, err
		}
		releases, err := client.Releases(ctx, name)
		if err != nil {
			return Summary{}, err
		}

		r := RepoStats{
			Name: name, Stars: info.Stars, Forks: info.Forks, Watchers: info.Watchers,
			OpenIssues: info.OpenIssuesAndPRs,
		}
		if prs, err := client.OpenPRCount(ctx, name); err == nil {
			r.OpenIssues -= prs
		}
		for _, rel := range releases {
			if rel.Draft {
				continue
			}
			for _, a := range rel.Assets {
				r.Downloads = append(r.Downloads, AssetDownloads{Tag: rel.TagName, Asset: a.Name, Count: a.DownloadCount})
			}
		}

		views, vErr := client.TrafficViews(ctx, name)
		clones, cErr := client.TrafficClones(ctx, name)
		refs, rErr := client.TrafficReferrers(ctx, name)
		paths, pErr := client.TrafficPaths(ctx, name)
		if vErr == nil && cErr == nil && rErr == nil && pErr == nil {
			r.TrafficOK = true
			r.Views, r.Clones = days(views), days(clones)
			for _, x := range refs {
				r.Referrers = append(r.Referrers, Hits{x.Referrer, x.Count, x.Uniques})
			}
			for _, x := range paths {
				r.Paths = append(r.Paths, Hits{x.Path, x.Count, x.Uniques})
			}
		}
		s.Repos = append(s.Repos, r)
	}
	return s, nil
}

func days(in []github.TrafficDay) []Day {
	out := make([]Day, 0, len(in))
	for _, d := range in {
		out = append(out, Day{d.Timestamp.UTC().Format("2006-01-02"), d.Count, d.Uniques})
	}
	return out
}
