package adoption

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

type fake struct{ trafficErr error }

func (fake) RepoInfo(context.Context, string) (github.RepoInfo, error) {
	return github.RepoInfo{Stars: 5, Forks: 1, Watchers: 2, OpenIssuesAndPRs: 4}, nil
}
func (fake) OpenPRCount(context.Context, string) (int, error) { return 3, nil }
func (fake) Releases(context.Context, string) ([]github.Release, error) {
	return []github.Release{
		{TagName: "v1", Assets: []github.ReleaseAsset{{Name: "a.tgz", DownloadCount: 7}}},
		{TagName: "v2", Draft: true, Assets: []github.ReleaseAsset{{Name: "b.tgz", DownloadCount: 1}}},
	}, nil
}
func (f fake) TrafficViews(context.Context, string) ([]github.TrafficDay, error) {
	return []github.TrafficDay{{Timestamp: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), Count: 9, Uniques: 4}}, f.trafficErr
}
func (fake) TrafficClones(context.Context, string) ([]github.TrafficDay, error) { return nil, nil }
func (fake) TrafficReferrers(context.Context, string) ([]github.Referrer, error) {
	return []github.Referrer{{Referrer: "google.com", Count: 3, Uniques: 2}}, nil
}
func (fake) TrafficPaths(context.Context, string) ([]github.PopularPath, error) { return nil, nil }

func TestBuild(t *testing.T) {
	s, err := Build(context.Background(), fake{}, []string{"delistd"})
	if err != nil {
		t.Fatal(err)
	}
	r := s.Repos[0]
	if r.OpenIssues != 1 || r.Stars != 5 {
		t.Fatalf("stats = %+v", r)
	}
	if len(r.Downloads) != 1 || r.Downloads[0].Tag != "v1" {
		t.Fatalf("drafts must be skipped: %+v", r.Downloads)
	}
	if !r.TrafficOK || r.Views[0].Date != "2026-11-13" || r.Referrers[0].Name != "google.com" {
		t.Fatalf("traffic = %+v", r)
	}
}

func TestBuildTrafficForbidden(t *testing.T) {
	s, err := Build(context.Background(), fake{trafficErr: errors.New("HTTP 403")}, []string{"delistd"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Repos[0].TrafficOK || s.Repos[0].Stars != 5 {
		t.Fatalf("%+v", s.Repos[0])
	}
}
