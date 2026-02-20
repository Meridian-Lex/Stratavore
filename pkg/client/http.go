package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/meridian-lex/stratavore/pkg/api"
	"go.uber.org/zap"
)

// Client communicates with stratavore daemon via HTTP API
type Client struct {
	baseURL string
	version int
	client  *http.Client
	logger  *zap.Logger
}

// NewClient creates a new API client
func NewClient(host string, port int, version int) *Client {
	logger, _ := zap.NewProduction()
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d/api/v%d", host, port, version),
		version: version,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// LaunchRunner launches a new runner
func (c *Client) LaunchRunner(ctx context.Context, req *api.LaunchRunnerRequest) (*api.LaunchRunnerResponse, error) {
	var resp api.LaunchRunnerResponse
	err := c.post(ctx, "/runners/launch", req, &resp)
	return &resp, err
}

// StopRunner stops a running runner
func (c *Client) StopRunner(ctx context.Context, runnerID string, force bool) (*api.StopRunnerResponse, error) {
	req := &api.StopRunnerRequest{
		RunnerID: runnerID,
		Force:    force,
	}
	var resp api.StopRunnerResponse
	err := c.post(ctx, "/runners/stop", req, &resp)
	return &resp, err
}

// GetRunner retrieves runner details
func (c *Client) GetRunner(ctx context.Context, runnerID string) (*api.GetRunnerResponse, error) {
	var resp api.GetRunnerResponse
	url := fmt.Sprintf("%s/runners/get?id=%s", c.baseURL, runnerID)
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// ListRunners lists active runners
func (c *Client) ListRunners(ctx context.Context, projectName string) (*api.ListRunnersResponse, error) {
	var resp api.ListRunnersResponse
	url := fmt.Sprintf("%s/runners/list", c.baseURL)
	if projectName != "" {
		url += fmt.Sprintf("?project=%s", projectName)
	}
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// CreateProject creates a new project
func (c *Client) CreateProject(ctx context.Context, req *api.CreateProjectRequest) (*api.CreateProjectResponse, error) {
	var resp api.CreateProjectResponse
	err := c.post(ctx, "/projects/create", req, &resp)
	return &resp, err
}

// ListProjects lists all projects
func (c *Client) ListProjects(ctx context.Context, status string) (*api.ListProjectsResponse, error) {
	var resp api.ListProjectsResponse
	url := fmt.Sprintf("%s/projects/list", c.baseURL)
	if status != "" {
		url += fmt.Sprintf("?status=%s", status)
	}
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// DeleteProject deletes a project
func (c *Client) DeleteProject(ctx context.Context, projectName string) (*api.DeleteProjectResponse, error) {
	req := &api.DeleteProjectRequest{
		Name: projectName,
	}
	var resp api.DeleteProjectResponse
	err := c.post(ctx, "/projects/delete", req, &resp)
	return &resp, err
}

// SendHeartbeat sends heartbeat from agent
func (c *Client) SendHeartbeat(ctx context.Context, req *api.HeartbeatRequest) (*api.HeartbeatResponse, error) {
	var resp api.HeartbeatResponse
	err := c.post(ctx, "/heartbeat", req, &resp)
	return &resp, err
}

// GetStatus retrieves daemon status
func (c *Client) GetStatus(ctx context.Context) (*api.GetStatusResponse, error) {
	var resp api.GetStatusResponse
	url := fmt.Sprintf("%s/status", c.baseURL)
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// ListSessions lists sessions for a project
func (c *Client) ListSessions(ctx context.Context, projectName string) (*api.ListSessionsResponse, error) {
	var resp api.ListSessionsResponse
	url := fmt.Sprintf("%s/sessions/list", c.baseURL)
	if projectName != "" {
		url += fmt.Sprintf("?project=%s", projectName)
	}
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// TriggerReconciliation manually triggers reconciliation
func (c *Client) TriggerReconciliation(ctx context.Context) (*api.TriggerReconciliationResponse, error) {
	var resp api.TriggerReconciliationResponse
	err := c.post(ctx, "/reconcile", nil, &resp)
	return &resp, err
}

// GetMode retrieves current operational mode
func (c *Client) GetMode(ctx context.Context) (*api.GetModeResponse, error) {
	var resp api.GetModeResponse
	url := fmt.Sprintf("%s/mode/get", c.baseURL)
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// SetMode updates operational mode
func (c *Client) SetMode(ctx context.Context, mode, description string) (*api.SetModeResponse, error) {
	req := &api.SetModeRequest{
		Mode:        mode,
		Description: description,
	}
	var resp api.SetModeResponse
	err := c.post(ctx, "/mode/set", req, &resp)
	return &resp, err
}

// GetConfig retrieves daemon configuration
func (c *Client) GetConfig(ctx context.Context) (*api.GetConfigResponse, error) {
	var resp api.GetConfigResponse
	url := fmt.Sprintf("%s/config", c.baseURL)
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// GetTokens retrieves token usage metrics
func (c *Client) GetTokens(ctx context.Context) (*api.GetTokensResponse, error) {
	var resp api.GetTokensResponse
	url := fmt.Sprintf("%s/tokens", c.baseURL)
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// GetState retrieves daemon state information
func (c *Client) GetState(ctx context.Context) (*api.GetStateResponse, error) {
	var resp api.GetStateResponse
	url := fmt.Sprintf("%s/state", c.baseURL)
	err := c.get(ctx, url, &resp)
	return &resp, err
}

// Helper methods

func (c *Client) post(ctx context.Context, path string, reqBody, respBody interface{}) error {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(errBody))
	}

	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

func (c *Client) get(ctx context.Context, url string, respBody interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(errBody))
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// Ping checks if daemon is reachable
func (c *Client) Ping(ctx context.Context) error {
	c.logger.Info("Pinging daemon", zap.String("url", c.baseURL+"/health"))

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		c.logger.Error("Failed to create HTTP request", zap.Error(err))
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("Failed to reach daemon", zap.Error(err))
		return fmt.Errorf("daemon unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("Daemon returned non-OK status", zap.Int("status", resp.StatusCode))
		return fmt.Errorf("daemon unhealthy: status %d", resp.StatusCode)
	}

	c.logger.Info("Daemon healthy")
	return nil
}
