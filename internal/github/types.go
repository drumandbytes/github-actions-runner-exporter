package github

import "time"

// Runner is the subset of GitHub's self-hosted runner object we use.
type Runner struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	OS     string  `json:"os"`
	Status string  `json:"status"` // "online" | "offline"
	Busy   bool    `json:"busy"`
	Labels []Label `json:"labels"`
}

type Label struct {
	Name string `json:"name"`
	Type string `json:"type"` // "read-only" | "custom"
}

type listRunnersResponse struct {
	TotalCount int      `json:"total_count"`
	Runners    []Runner `json:"runners"`
}

// Repo is the subset of GitHub's repository object this exporter needs.
type Repo struct {
	Name     string `json:"name"`
	Private  bool   `json:"private"`
	Archived bool   `json:"archived"`
}

// Workflow is a repo's workflow definition (not a run of it).
type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"` // "active" | "disabled_manually" | "disabled_inactivity" | ...
}

type listWorkflowsResponse struct {
	TotalCount int        `json:"total_count"`
	Workflows  []Workflow `json:"workflows"`
}

// WorkflowRun is the subset of a workflow run we use.
type WorkflowRun struct {
	ID         int64     `json:"id"`
	WorkflowID int64     `json:"workflow_id"`
	Name       string    `json:"name"` // the workflow's name, not the run-name title
	RunAttempt int       `json:"run_attempt"`
	Status     string    `json:"status"`     // "completed" | "in_progress" | ...
	Conclusion string    `json:"conclusion"` // "success" | "failure" | ... (empty until completed)
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HTMLURL    string    `json:"html_url"`
}

type listWorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

// Job is the subset of a workflow job we use. Zero times mean GitHub sent null.
type Job struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Conclusion      string    `json:"conclusion"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	RunnerName      string    `json:"runner_name"`
	RunnerGroupName string    `json:"runner_group_name"`
	Labels          []string  `json:"labels"` // runs-on labels
}

type listJobsResponse struct {
	TotalCount int   `json:"total_count"`
	Jobs       []Job `json:"jobs"`
}

// DependabotAlert is the subset of a Dependabot alert this exporter needs.
type DependabotAlert struct {
	State            string `json:"state"` // "open" | "fixed" | "dismissed" | "auto_dismissed"
	SecurityAdvisory struct {
		Severity string `json:"severity"` // "low" | "medium" | "high" | "critical"
	} `json:"security_advisory"`
}

// RateLimit is the "core" resource from GET /rate_limit.
type RateLimit struct {
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
}

type rateLimitResponse struct {
	Resources struct {
		Core RateLimit `json:"core"`
	} `json:"resources"`
}
