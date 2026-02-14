package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/config"
	"go.uber.org/zap"
)

// githubPR is the subset of GitHub's PR response we need.
type githubPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Draft     bool   `json:"draft"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
}

// FleetHandler fetches open PRs from GitHub and caches them in memory.
type FleetHandler struct {
	token    string
	repos    []string
	logger   *zap.Logger
	cacheTTL time.Duration

	mu       sync.Mutex
	cached   []api.FleetPR
	cachedAt time.Time
}

// NewFleetHandler creates a FleetHandler from config.
func NewFleetHandler(cfg config.GitHubConfig, logger *zap.Logger) *FleetHandler {
	repos := cfg.FleetRepos
	if len(repos) == 0 {
		repos = []string{
			"Meridian-Lex/Stratavore",
			"Meridian-Lex/Gantry",
			"Meridian-Lex/Lex-webui",
			"Meridian-Lex/lex-state",
		}
	}
	return &FleetHandler{
		token:    cfg.Token,
		repos:    repos,
		logger:   logger,
		cacheTTL: 120 * time.Second,
	}
}

// GetPRs returns cached PRs or fetches fresh ones from GitHub.
// If refresh is true, bypasses the cache.
func (h *FleetHandler) GetPRs(ctx context.Context, refresh bool) ([]api.FleetPR, time.Time, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !refresh && h.cached != nil && time.Since(h.cachedAt) < h.cacheTTL {
		return h.cached, h.cachedAt, nil
	}

	prs, err := h.fetchFromGitHub(ctx)
	if err != nil {
		// Return stale cache on error rather than failing
		if h.cached != nil {
			h.logger.Warn("GitHub fetch failed, returning stale cache", zap.Error(err))
			return h.cached, h.cachedAt, nil
		}
		return nil, time.Time{}, err
	}

	h.cached = prs
	h.cachedAt = time.Now()
	return h.cached, h.cachedAt, nil
}

// fetchFromGitHub calls the GitHub REST API for each fleet repo.
func (h *FleetHandler) fetchFromGitHub(ctx context.Context) ([]api.FleetPR, error) {
	if h.token == "" {
		return nil, fmt.Errorf("github.token not configured")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var allPRs []api.FleetPR

	for _, repo := range h.repos {
		url := fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=open&per_page=100", repo)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			h.logger.Warn("failed to build GitHub request", zap.String("repo", repo), zap.Error(err))
			continue
		}
		req.Header.Set("Authorization", "Bearer "+h.token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := client.Do(req)
		if err != nil {
			h.logger.Warn("GitHub request failed", zap.String("repo", repo), zap.Error(err))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			h.logger.Warn("GitHub non-200 response",
				zap.String("repo", repo),
				zap.Int("status", resp.StatusCode))
			continue
		}

		var prs []githubPR
		if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
			h.logger.Warn("failed to decode GitHub response", zap.String("repo", repo), zap.Error(err))
			continue
		}

		for _, pr := range prs {
			assignees := make([]string, len(pr.Assignees))
			for i, a := range pr.Assignees {
				assignees[i] = a.Login
			}
			createdAt, _ := time.Parse(time.RFC3339, pr.CreatedAt)
			allPRs = append(allPRs, api.FleetPR{
				Repo:      repo,
				Number:    pr.Number,
				Title:     pr.Title,
				Author:    pr.User.Login,
				CreatedAt: createdAt,
				Draft:     pr.Draft,
				Assignees: assignees,
				URL:       pr.HTMLURL,
			})
		}
	}

	return allPRs, nil
}
