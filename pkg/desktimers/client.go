package desktimers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnauthorized is returned when the API rejects the stored token; the
// caller should trigger a fresh device-flow login.
var ErrUnauthorized = errors.New("desktimers: unauthorized (token invalid or revoked)")

// Client talks to the DeskTimers git-client API.
type Client struct {
	BaseURL     string
	AccessToken string
	HTTPClient  *http.Client
}

// NewClient builds a client for the given base URL and access token.
func NewClient(baseURL, accessToken string) *Client {
	return &Client{
		BaseURL:     strings.TrimSuffix(baseURL, "/"),
		AccessToken: accessToken,
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientFromToken builds a client from the stored token. Returns
// ErrUnauthorized when no usable token is stored.
func NewClientFromToken() (*Client, error) {
	token, err := LoadToken()
	if err != nil {
		return nil, err
	}
	if !token.Valid() {
		return nil, ErrUnauthorized
	}
	return NewClient(ResolveBaseURL(), token.AccessToken), nil
}

type tasksResponse struct {
	Success bool   `json:"success"`
	Data    []Task `json:"data"`
}

// GetTasks fetches the user's assigned tasks, optionally filtered by status
// (e.g. "active").
func (c *Client) GetTasks(status string) ([]Task, error) {
	endpoint := c.BaseURL + "/api/git-client/tasks"
	if status != "" {
		endpoint += "?status=" + url.QueryEscape(status)
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building tasks request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching tasks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching tasks: unexpected status %d", resp.StatusCode)
	}
	parsed := tasksResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing tasks response: %w", err)
	}
	return parsed.Data, nil
}

// Project is a DeskTimers project usable for quick task creation (only
// projects with a code are returned by the API).
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Workspace string `json:"workspace"`
}

type projectsResponse struct {
	Success bool      `json:"success"`
	Data    []Project `json:"data"`
}

// GetProjects fetches the projects the user can create tasks in.
func (c *Client) GetProjects() ([]Project, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/git-client/projects", nil)
	if err != nil {
		return nil, fmt.Errorf("building projects request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching projects: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching projects: unexpected status %d", resp.StatusCode)
	}
	parsed := projectsResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing projects response: %w", err)
	}
	return parsed.Data, nil
}

type createTaskResponse struct {
	Success bool   `json:"success"`
	Data    Task   `json:"data"`
	Message string `json:"message"`
}

// CreateTask creates a task in the given project and returns it in the same
// shape the task list uses, ready to be selected.
func (c *Client) CreateTask(projectID string, title string) (Task, error) {
	body, err := json.Marshal(map[string]string{"projectId": projectID, "title": title})
	if err != nil {
		return Task{}, fmt.Errorf("encoding create-task request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/git-client/tasks", bytes.NewReader(body))
	if err != nil {
		return Task{}, fmt.Errorf("building create-task request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Task{}, fmt.Errorf("creating task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return Task{}, ErrUnauthorized
	}
	parsed := createTaskResponse{}
	decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
	if resp.StatusCode != http.StatusOK {
		if decodeErr == nil && parsed.Message != "" {
			return Task{}, fmt.Errorf("creating task: %s", parsed.Message)
		}
		return Task{}, fmt.Errorf("creating task: unexpected status %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return Task{}, fmt.Errorf("parsing create-task response: %w", decodeErr)
	}
	return parsed.Data, nil
}

// Column represents a project board column returned by the API.
type Column struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Position int    `json:"position"`
}

type columnsResponse struct {
	Success bool     `json:"success"`
	Data    []Column `json:"data"`
}

// GetTaskColumns returns the ordered column list for the project that owns
// the task identified by code (e.g. "DESK-296").
func (c *Client) GetTaskColumns(code string) ([]Column, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/git-client/tasks/"+code+"/columns", nil)
	if err != nil {
		return nil, fmt.Errorf("building columns request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching columns: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching columns: unexpected status %d", resp.StatusCode)
	}
	parsed := columnsResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing columns response: %w", err)
	}
	return parsed.Data, nil
}

// MoveTaskToColumn moves a task to the given column slug and stops the active
// timer if the column is not in_progress.
func (c *Client) MoveTaskToColumn(code string, columnSlug string) error {
	body, err := json.Marshal(map[string]string{"column": columnSlug})
	if err != nil {
		return fmt.Errorf("encoding move-task request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.BaseURL+"/api/git-client/tasks/"+code, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building move-task request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("moving task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("moving task: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// SuppressAutoDone tells the backend to skip the webhook-driven move-to-Done
// for this task. Call this BEFORE merging the PR when the user answers "Merge
// only" — the flag is stored in Redis for 5 minutes, long enough to outlive
// GitHub's webhook delivery.
func (c *Client) SuppressAutoDone(code string) error {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/git-client/tasks/"+code+"/suppress-done", nil)
	if err != nil {
		return fmt.Errorf("building suppress-done request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("suppress-done: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("suppress-done: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// SetTaskStatus updates a task's status via PATCH /api/git-client/tasks/{code}.
// Common values: "done", "in_progress".
func (c *Client) SetTaskStatus(code string, status string) error {
	body, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return fmt.Errorf("encoding set-status request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.BaseURL+"/api/git-client/tasks/"+code, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building set-status request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("setting task status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("setting task status: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// MoveTaskToDone marks a task as done.
func (c *Client) MoveTaskToDone(code string) error {
	return c.SetTaskStatus(code, "done")
}

// MoveTaskToInProgress reverts a task back to in_progress — used when the
// GitHub webhook auto-moved it to Done but the user said "No" in deskgit.
func (c *Client) MoveTaskToInProgress(code string) error {
	return c.SetTaskStatus(code, "in_progress")
}
