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
