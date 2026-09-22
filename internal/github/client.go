// Package github is a minimal client for the one GitHub REST API call
// this exporter needs: listing an organization's self-hosted runners.
// It deliberately does not try to be a general-purpose GitHub API client.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const apiBase = "https://api.github.com"

type Client struct {
	org        string
	token      string
	httpClient *http.Client
}

func NewClient(org, token string, timeout time.Duration) *Client {
	return &Client{
		org:        org,
		token:      token,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Runners returns every self-hosted runner registered at the
// organization level. GitHub paginates this endpoint at 30 per page by
// default; a homelab-sized org with a handful of runners never needs a
// second page, so pagination is deliberately not implemented here -
// add it if this ever needs to scale past ~100 runners.
func (c *Client) Runners(ctx context.Context) ([]Runner, error) {
	url := fmt.Sprintf("%s/orgs/%s/actions/runners?per_page=100", apiBase, c.org)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}

	var out listRunnersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return out.Runners, nil
}
