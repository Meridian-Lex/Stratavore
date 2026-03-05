package api

import "time"

// FleetPR represents a single open pull request across fleet repositories.
// PRs are aggregated from Meridian-Lex repositories via GitHub API and cached
// for dashboard display. Only open (non-draft) PRs are included by default.
type FleetPR struct {
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	Draft     bool      `json:"draft"`
	Assignees []string  `json:"assignees"`
	URL       string    `json:"url"`
}

// FleetPRsResponse is returned by GET /api/v1/fleet/prs.
// CachedAt indicates when the PR list was last fetched from GitHub.
// Total reflects the count of PRs in the response, not a GitHub total.
type FleetPRsResponse struct {
	PRs      []FleetPR `json:"prs"`
	CachedAt time.Time `json:"cached_at"`
	Total    int       `json:"total"`
}

// Len returns the number of PRs in the response. It is a convenience
// accessor over len(PRs) that mirrors the Total field.
func (r FleetPRsResponse) Len() int {
	return len(r.PRs)
}
