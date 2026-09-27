// Package runners builds the runner status summary, on a much shorter TTL than orgstats.
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
