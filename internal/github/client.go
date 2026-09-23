// Package github is a minimal client for the GitHub REST endpoints this
// exporter needs. It deliberately does not try to be a general-purpose
// GitHub API client.
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

// Runners returns every self-hosted runner registered at the
// organization level. GitHub paginates this endpoint at 30 per page by
// default; a homelab-sized org with a handful of runners never needs a
// second page, so pagination is deliberately not implemented here -
// add it if this ever needs to scale past 100 runners.
func (c *Client) Runners(ctx context.Context) ([]Runner, error) {
	var out listRunnersResponse
	url := fmt.Sprintf("%s/orgs/%s/actions/runners?per_page=100", apiBase, c.org)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out.Runners, nil
}

// Repos returns every non-forked repo in the org. Capped at 100 - this
// org has a few dozen, well under GitHub's page size, so pagination is
// deliberately not implemented; add it if the org ever grows past 100
// repos.
func (c *Client) Repos(ctx context.Context) ([]Repo, error) {
	var out []Repo
	url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100&type=all", apiBase, c.org)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Workflows returns a repo's workflow definitions. Capped at 100 - no
// repo here plausibly has more than that many workflow files; add
// pagination if that ever changes.
func (c *Client) Workflows(ctx context.Context, repo string) ([]Workflow, error) {
	var out listWorkflowsResponse
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows?per_page=100", apiBase, c.org, repo)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out.Workflows, nil
}

// LatestRunForWorkflow returns the most recently created run of one
// specific workflow. Checking per-workflow, rather than the single
// most recent run across a repo's whole Actions history, matters: a
// repo with several workflows (e.g. a Validate that runs on PRs and a
// Build that runs on push) would otherwise report whichever one
// happened to run last as if it spoke for all of them - a failing
// Validate can sit hidden behind a later, unrelated, successful Build.
// Returns ok=false if this workflow has never run.
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

// OpenPRCount returns the number of open pull requests in a repo,
// without paginating through them: it reads the page count off the
// Link response header of a per_page=1 request instead. If there's no
// "last" rel (0 or 1 open PRs), the length of the single returned page
// is the count.
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

// DependabotAlerts returns every open Dependabot alert for a repo.
// Capped at 100 - a homelab-scale repo realistically never has more
// open alerts than that; a repo that somehow did would just undercount
// here rather than error.
func (c *Client) DependabotAlerts(ctx context.Context, repo string) ([]DependabotAlert, error) {
	var out []DependabotAlert
	url := fmt.Sprintf("%s/repos/%s/%s/dependabot/alerts?state=open&per_page=100", apiBase, c.org, repo)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RateLimit returns the core API rate limit budget this client itself
// draws from.
func (c *Client) RateLimit(ctx context.Context) (RateLimit, error) {
	var out rateLimitResponse
	if err := c.get(ctx, apiBase+"/rate_limit", &out); err != nil {
		return RateLimit{}, err
	}
	return out.Resources.Core, nil
}
