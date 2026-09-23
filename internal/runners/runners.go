// Package runners builds the self-hosted-runner status summary - kept
// separate from orgstats because it's fetched on a much shorter cache
// TTL (runner up/busy is genuinely real-time; CI/PR/Dependabot signals
// are not, and polling them that often would blow through GitHub's
// rate limit across two dozen repos).
package runners

import (
	"context"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

type Runner struct {
	Name string
	OS   string
	Up   bool
	Busy bool
}

type Summary struct {
	GeneratedAt time.Time
	Runners     []Runner
}

func Build(ctx context.Context, client *github.Client) (Summary, error) {
	runners, err := client.Runners(ctx)
	if err != nil {
		return Summary{}, err
	}

	s := Summary{GeneratedAt: time.Now()}
	for _, r := range runners {
		s.Runners = append(s.Runners, Runner{
			Name: r.Name,
			OS:   r.OS,
			Up:   r.Status == "online",
			Busy: r.Busy,
		})
	}
	return s, nil
}
