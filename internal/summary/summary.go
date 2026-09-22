// Package summary flattens the raw GitHub API response into the shape
// the Prometheus collector needs.
package summary

import (
	"context"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

type RunnerSummary struct {
	Name string
	OS   string
	Up   bool
	Busy bool
}

type Summary struct {
	GeneratedAt time.Time
	Runners     []RunnerSummary
}

func Build(ctx context.Context, client *github.Client) (Summary, error) {
	runners, err := client.Runners(ctx)
	if err != nil {
		return Summary{}, err
	}

	s := Summary{GeneratedAt: time.Now()}
	for _, r := range runners {
		s.Runners = append(s.Runners, RunnerSummary{
			Name: r.Name,
			OS:   r.OS,
			Up:   r.Status == "online",
			Busy: r.Busy,
		})
	}
	return s, nil
}
