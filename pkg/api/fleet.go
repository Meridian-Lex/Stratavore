package api

import "time"

// FleetPR represents a single open pull request across fleet repositories.
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
type FleetPRsResponse struct {
	PRs      []FleetPR `json:"prs"`
	CachedAt time.Time `json:"cached_at"`
	Total    int       `json:"total"`
}
