// Package github is a minimal client for the GitHub endpoints this exporter uses.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const apiBase = "https://api.github.com"

type Client struct {
	baseURL    string // apiBase; tests point it at httptest
	now        func() time.Time
	org        string
	token      string
	httpClient *http.Client

	mu sync.Mutex // both pollers share the client
	// core budgets by reset time: GitHub counts per region, so one token has
	// more than one window at once, depending on which region serves an endpoint
	budgets map[int64]RateLimit
}

func NewClient(org, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    apiBase,
		now:        time.Now,
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
	c.recordRateLimit(resp.Header)
	return resp, nil
}

// recordRateLimit keeps the core budget from a response's headers. GET
// /rate_limit can't be trusted for this: it reports used=0 for our tokens
// while these headers show the real count.
func (c *Client) recordRateLimit(h http.Header) {
	if r := h.Get("X-RateLimit-Resource"); r != "" && r != "core" {
		return
	}
	limit, lErr := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, rErr := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, sErr := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if lErr != nil || rErr != nil || sErr != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.budgets == nil {
		c.budgets = map[int64]RateLimit{}
	}
	// within one window, the lowest count seen is the latest
	if b, ok := c.budgets[reset]; !ok || remaining < b.Remaining {
		c.budgets[reset] = RateLimit{Limit: limit, Remaining: remaining}
	}
}

func (c *Client) get(ctx context.Context, url string, out interface{}) error {
	_, err := c.getPage(ctx, url, out)
	return err
}

var nextPageRegexp = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// getPage decodes one page into out and returns the Link header's "next" URL, "" on the last page.
func (c *Client) getPage(ctx context.Context, url string, out interface{}) (next string, err error) {
	resp, err := c.request(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return "", fmt.Errorf("decoding response from %s: %w", url, err)
	}
	if m := nextPageRegexp.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		return m[1], nil
	}
	return "", nil
}

// Runners returns the org's self-hosted runners. No pagination; add it past 100.
func (c *Client) Runners(ctx context.Context) ([]Runner, error) {
	var out listRunnersResponse
	url := fmt.Sprintf("%s/orgs/%s/actions/runners?per_page=100", c.baseURL, c.org)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out.Runners, nil
}

// Repos returns the org's non-fork repos. No pagination; add it past 100.
func (c *Client) Repos(ctx context.Context) ([]Repo, error) {
	var out []Repo
	url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100&type=all", c.baseURL, c.org)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Workflows returns a repo's workflow definitions. No pagination; add it past 100.
func (c *Client) Workflows(ctx context.Context, repo string) ([]Workflow, error) {
	var out listWorkflowsResponse
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows?per_page=100", c.baseURL, c.org, repo)
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
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%d/runs?per_page=1", c.baseURL, c.org, repo, workflowID)
	if err := c.get(ctx, url, &out); err != nil {
		return WorkflowRun{}, false, err
	}
	if len(out.WorkflowRuns) == 0 {
		return WorkflowRun{}, false, nil
	}
	return out.WorkflowRuns[0], true, nil
}

// RunsCreatedSince returns every run (any status) created at or after since,
// following pagination. Not filtered to completed: a long run created before
// a faster later one must stay visible until it finishes.
func (c *Client) RunsCreatedSince(ctx context.Context, repo string, since time.Time) ([]WorkflowRun, error) {
	next := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=100&created=%s",
		c.baseURL, c.org, repo, url.QueryEscape(">="+since.UTC().Format(time.RFC3339)))
	var runs []WorkflowRun
	for next != "" {
		var out listWorkflowRunsResponse
		var err error
		if next, err = c.getPage(ctx, next, &out); err != nil {
			return nil, err
		}
		runs = append(runs, out.WorkflowRuns...)
	}
	return runs, nil
}

// RunJobs returns the jobs of a run's latest attempt, following pagination.
func (c *Client) RunJobs(ctx context.Context, repo string, runID int64) ([]Job, error) {
	next := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/jobs?per_page=100", c.baseURL, c.org, repo, runID)
	var jobs []Job
	for next != "" {
		var out listJobsResponse
		var err error
		if next, err = c.getPage(ctx, next, &out); err != nil {
			return nil, err
		}
		jobs = append(jobs, out.Jobs...)
	}
	return jobs, nil
}

var lastPageRegexp = regexp.MustCompile(`[?&]page=(\d+)>;\s*rel="last"`)

// OpenPRCount reads the count off the Link header's "last" page of a
// per_page=1 request; with no "last" rel it's the page length.
func (c *Client) OpenPRCount(ctx context.Context, repo string) (int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=1", c.baseURL, c.org, repo)
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
	url := fmt.Sprintf("%s/repos/%s/%s/dependabot/alerts?state=open&per_page=100", c.baseURL, c.org, repo)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RateLimits returns every core window that hasn't reset yet, by reset time.
func (c *Client) RateLimits() map[time.Time]RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now().Unix()
	out := map[time.Time]RateLimit{}
	for reset, b := range c.budgets {
		if reset <= now {
			delete(c.budgets, reset)
			continue
		}
		out[time.Unix(reset, 0).UTC()] = b
	}
	return out
}
