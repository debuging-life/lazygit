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
	return NewClient(ResolveBaseURL(token), token.AccessToken), nil
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
