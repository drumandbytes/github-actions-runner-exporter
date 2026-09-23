package github

// Runner is the subset of GitHub's self-hosted runner object this
// exporter cares about. See:
// https://docs.github.com/en/rest/actions/self-hosted-runners#list-self-hosted-runners-for-an-organization
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

// WorkflowRun is the subset of a workflow run this exporter needs to
// derive "is CI currently green" for one workflow.
type WorkflowRun struct {
	Status     string `json:"status"`     // "completed" | "in_progress" | ...
	Conclusion string `json:"conclusion"` // "success" | "failure" | ... (empty until completed)
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	HTMLURL    string `json:"html_url"`
}

type listWorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

// DependabotAlert is the subset of a Dependabot alert this exporter needs.
type DependabotAlert struct {
	State            string `json:"state"` // "open" | "fixed" | "dismissed" | "auto_dismissed"
	SecurityAdvisory struct {
		Severity string `json:"severity"` // "low" | "medium" | "high" | "critical"
	} `json:"security_advisory"`
}

// RateLimit is the "core" resource from GET /rate_limit - the budget
// every other call in this client draws from.
type RateLimit struct {
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
}

type rateLimitResponse struct {
	Resources struct {
		Core RateLimit `json:"core"`
	} `json:"resources"`
}
