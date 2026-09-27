// Package github is a minimal client for the GitHub endpoints this exporter uses.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
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

func (c *Client) request(ctx context.Context, url string) (*http.Response, error) {
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
	return resp, nil
}

func (c *Client) get(ctx context.Context, url string, out interface{}) error {
	resp, err := c.request(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return nil
}

// Runners returns the org's self-hosted runners. No pagination; add it past 100.
func (c *Client) Runners(ctx context.Context) ([]Runner, error) {
	var out listRunnersResponse
	url := fmt.Sprintf("%s/orgs/%s/actions/runners?per_page=100", apiBase, c.org)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out.Runners, nil
}

// Repos returns the org's non-fork repos. No pagination; add it past 100.
func (c *Client) Repos(ctx context.Context) ([]Repo, error) {
	var out []Repo
	url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100&type=all", apiBase, c.org)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Workflows returns a repo's workflow definitions. No pagination; add it past 100.
func (c *Client) Workflows(ctx context.Context, repo string) ([]Workflow, error) {
	var out listWorkflowsResponse
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows?per_page=100", apiBase, c.org, repo)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out.Workflows, nil
}

// LatestRunForWorkflow returns one workflow's most recent run, ok=false if it
// never ran. Per workflow, so a failing Validate can't hide behind a later
// green Build in the same repo.
func (c *Client) LatestRunForWorkflow(ctx context.Context, repo string, workflowID int64) (run WorkflowRun, ok bool, err error) {
	var out listWorkflowRunsResponse
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%d/runs?per_page=1", apiBase, c.org, repo, workflowID)
	if err := c.get(ctx, url, &out); err != nil {
		return WorkflowRun{}, false, err
	}
	if len(out.WorkflowRuns) == 0 {
		return WorkflowRun{}, false, nil
	}
	return out.WorkflowRuns[0], true, nil
}

var lastPageRegexp = regexp.MustCompile(`[?&]page=(\d+)>;\s*rel="last"`)

// OpenPRCount reads the count off the Link header's "last" page of a
// per_page=1 request; with no "last" rel it's the page length.
func (c *Client) OpenPRCount(ctx context.Context, repo string) (int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=1", apiBase, c.org, repo)
	resp, err := c.request(ctx, url)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}

	if m := lastPageRegexp.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("parsing last page number from Link header: %w", err)
		}
		return n, nil
	}

	var page []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return 0, fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return len(page), nil
}

// DependabotAlerts returns a repo's open alerts. Capped at 100; undercounts past that.
func (c *Client) DependabotAlerts(ctx context.Context, repo string) ([]DependabotAlert, error) {
	var out []DependabotAlert
	url := fmt.Sprintf("%s/repos/%s/%s/dependabot/alerts?state=open&per_page=100", apiBase, c.org, repo)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RateLimit returns the core rate-limit budget this client draws from.
func (c *Client) RateLimit(ctx context.Context) (RateLimit, error) {
	var out rateLimitResponse
	if err := c.get(ctx, apiBase+"/rate_limit", &out); err != nil {
		return RateLimit{}, err
	}
	return out.Resources.Core, nil
}
