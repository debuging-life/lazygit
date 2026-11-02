package desktimers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrUnauthorized is returned when the API rejects the stored token (401).
// Callers should surface a "re-login" hint.
var ErrUnauthorized = errors.New("desktick: not signed in (the session ended or was signed out)")

// ── the DeskTick API ────────────────────────────────────────────────────────
// deskgit has no API of its own. It signs in as the person (auth.go) and
// asks the routes the web and desktop apps already use, so what it shows is
// what they show and what it may do is what the person may do:
//
//	GET  /api/workspaces                                  the workspaces I am in
//	GET  /api/workspaces/:ws/projects?memberId=me         the projects I am ON
//	GET  /api/workspaces/:ws/projects/:code/board         its columns and my cards
//	POST /api/workspaces/:ws/projects/:code/tasks         a new task
//	POST /api/workspaces/:ws/projects/:code/tasks/:task/move
//	GET  /api/workspaces/:ws/projects/:code/repositories  the repositories linked to it
//	GET  /api/workspaces/:ws/time/timer/current           what my timer is on
//	GET  /api/workspaces/:ws/time/summary                 my seconds, a day at a time
//	POST /api/github/commits                              commits as they are made (commits.go)
//
// A project is addressed by its CODE ("CLK") and a task by its code
// ("CLK-12"). A project code is unique in a workspace and not across them,
// so a project is carried here as a Place: the workspace and the code.

// Client talks to DeskTick as the signed-in person.
type Client struct {
	// BaseURL is the web app's address; the API is under /api.
	BaseURL     string
	AccessToken string
	// UserID is the signed-in person: what "mine" is asked for by.
	UserID     string
	HTTPClient *http.Client
}

// NewClient builds a client for the given site and bearer token.
func NewClient(baseURL, accessToken string) *Client {
	return &Client{
		BaseURL:     strings.TrimSuffix(baseURL, "/"),
		AccessToken: accessToken,
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientFromToken builds a client from the stored token. Returns
// ErrUnauthorized when there is no usable token.
func NewClientFromToken() (*Client, error) {
	token, err := LoadToken()
	if err != nil {
		return nil, err
	}
	if !token.Valid() {
		return nil, ErrUnauthorized
	}
	client := NewClient(ResolveBaseURL(), token.AccessToken)
	client.UserID = token.UserID
	return client, nil
}

// StatusError is the API saying no, in its own words when it gave any.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("DeskTick answered with status %d", e.Status)
}

// call makes one request under /api and reads the `data` of the reply into
// `out` (nil to ignore it).
func (c *Client) call(method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding the request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, c.BaseURL+"/api"+path, reader)
	if err != nil {
		return fmt.Errorf("building the request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("User-Agent", "deskgit/"+Version)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("reaching DeskTick: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}

	envelope := struct {
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
	}{}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{Status: resp.StatusCode, Message: envelope.Message}
	}
	if out == nil {
		return nil
	}
	if decodeErr != nil {
		return fmt.Errorf("reading DeskTick's answer: %w", decodeErr)
	}
	if len(envelope.Data) == 0 {
		return errors.New("DeskTick's answer had nothing in it")
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("reading DeskTick's answer: %w", err)
	}
	return nil
}

// ── where things are ────────────────────────────────────────────────────────

type workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Place is a project, with the workspace it is in.
type Place struct {
	WorkspaceID   string
	WorkspaceSlug string
	WorkspaceName string
	ProjectCode   string
	ProjectName   string
}

// key is how a Place is carried through the pickers as a Project's ID.
func (p Place) key() string { return p.WorkspaceID + "/" + p.ProjectCode }

func (c *Client) workspaces() ([]workspace, error) {
	list := []workspace{}
	if err := c.call(http.MethodGet, "/workspaces", nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// maxProjectPages bounds the walk through a workspace's projects.
const maxProjectPages = 20

// myProjects is the projects the person is ON in one workspace: not every
// project they are allowed to see. An owner can see the whole workspace; a
// commit is for the work that is theirs.
func (c *Client) myProjects(ws workspace) ([]Place, error) {
	places := []Place{}
	for page := 1; page <= maxProjectPages; page++ {
		got := struct {
			Items []struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"items"`
			Total int `json:"total"`
		}{}
		query := url.Values{"status": {"active"}, "page": {fmt.Sprint(page)}, "memberId": {c.UserID}}
		if err := c.call(http.MethodGet, "/workspaces/"+url.PathEscape(ws.ID)+"/projects?"+query.Encode(), nil, &got); err != nil {
			return nil, err
		}
		for _, item := range got.Items {
			places = append(places, Place{
				WorkspaceID: ws.ID, WorkspaceSlug: ws.Slug, WorkspaceName: ws.Name,
				ProjectCode: item.Code, ProjectName: item.Name,
			})
		}
		if len(got.Items) == 0 || len(places) >= got.Total {
			break
		}
	}
	return places, nil
}

// each runs `work` once per item, a few at a time, and returns the first
// error. The answers are the caller's to collect; `work` is given the index.
func each(count int, work func(i int) error) error {
	const atOnce = 6
	gate := make(chan struct{}, atOnce)
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	for i := range count {
		wg.Add(1)
		gate <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-gate }()
			if err := work(i); err != nil {
				once.Do(func() { first = err })
			}
		}(i)
	}
	wg.Wait()
	return first
}

// places is every project the person is on, in every workspace, in a stable
// order (workspace name, then project name).
func (c *Client) places() ([]Place, error) {
	workspaces, err := c.workspaces()
	if err != nil {
		return nil, err
	}
	perWorkspace := make([][]Place, len(workspaces))
	if err := each(len(workspaces), func(i int) error {
		got, err := c.myProjects(workspaces[i])
		perWorkspace[i] = got
		return err
	}); err != nil {
		return nil, err
	}
	all := []Place{}
	for _, got := range perWorkspace {
		all = append(all, got...)
	}
	sort.SliceStable(all, func(a, b int) bool {
		if all[a].WorkspaceName != all[b].WorkspaceName {
			return all[a].WorkspaceName < all[b].WorkspaceName
		}
		return all[a].ProjectName < all[b].ProjectName
	})
	return all, nil
}

// knownPlaces remembers where each task that has been listed lives, so that
// "move CLK-12" does not have to ask where CLK is again. A task that was
// never listed is found by its project code (placeOf).
var knownPlaces = struct {
	sync.Mutex
	byTask map[string]Place
}{byTask: map[string]Place{}}

func remember(code string, place Place) {
	knownPlaces.Lock()
	defer knownPlaces.Unlock()
	knownPlaces.byTask[strings.ToUpper(code)] = place
}

// ProjectCodeOf is the project part of a task code: "CLK" of "CLK-12".
func ProjectCodeOf(taskCode string) string {
	cut := strings.LastIndex(taskCode, "-")
	if cut <= 0 {
		return ""
	}
	return strings.ToUpper(taskCode[:cut])
}

// placeOf is the project a task code belongs to.
func (c *Client) placeOf(taskCode string) (Place, error) {
	knownPlaces.Lock()
	place, ok := knownPlaces.byTask[strings.ToUpper(taskCode)]
	knownPlaces.Unlock()
	if ok {
		return place, nil
	}
	project := ProjectCodeOf(taskCode)
	places, err := c.places()
	if err != nil {
		return Place{}, err
	}
	for _, candidate := range places {
		if strings.EqualFold(candidate.ProjectCode, project) {
			return candidate, nil
		}
	}
	return Place{}, fmt.Errorf("%s is not in a project you are on", taskCode)
}

func (p Place) path() string {
	return "/workspaces/" + url.PathEscape(p.WorkspaceID) + "/projects/" + url.PathEscape(p.ProjectCode)
}

// TaskURL is the task's page in the web app.
func (c *Client) TaskURL(place Place, taskCode string) string {
	return fmt.Sprintf("%s/w/%s/projects/%s/tasks/%s",
		c.BaseURL, url.PathEscape(place.WorkspaceSlug), url.PathEscape(place.ProjectCode), url.PathEscape(taskCode))
}

// ── the board ───────────────────────────────────────────────────────────────

type boardColumn struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	// IsDone marks a column whose tasks are finished. A server that does not
	// send it yet leaves it false, and every column reads as open.
	IsDone bool `json:"isDone"`
	Tasks  []struct {
		Code  string `json:"code"`
		Title string `json:"title"`
	} `json:"tasks"`
}

// board is a project's columns in order, with the cards asked for: the
// person's own when `mine`, otherwise whatever they may see.
func (c *Client) board(place Place, mine bool) ([]boardColumn, error) {
	got := struct {
		Columns []boardColumn `json:"columns"`
	}{}
	path := place.path() + "/board"
	if mine {
		path += "?" + url.Values{"assigneeId": {c.UserID}}.Encode()
	}
	if err := c.call(http.MethodGet, path, nil, &got); err != nil {
		return nil, err
	}
	sort.SliceStable(got.Columns, func(a, b int) bool { return got.Columns[a].Position < got.Columns[b].Position })
	return got.Columns, nil
}

// StatusTodo is a task that has not been started.
const StatusTodo = "todo"

// statusOf is what a column says about the tasks in it. DeskTick's columns
// are the workspace's own, with any names, so this goes by where a column
// IS: the first open one is where work waits, and any open one after it is
// work under way. (The project page's overview counts them the same way.)
func statusOf(columns []boardColumn, index int) string {
	if columns[index].IsDone {
		return "done"
	}
	for i := range index {
		if !columns[i].IsDone {
			return StatusInProgress
		}
	}
	return StatusTodo
}

// tracking is the task code the person's timer is on, per workspace id.
func (c *Client) tracking(workspaceID string) (string, time.Time, error) {
	// `data` is null when nothing is running, which `call` reads as "nothing
	// in it"; a pointer tells the two apart.
	var current *struct {
		TaskCode  string    `json:"taskCode"`
		StartedAt time.Time `json:"startedAt"`
	}
	raw := json.RawMessage{}
	if err := c.call(http.MethodGet, "/workspaces/"+url.PathEscape(workspaceID)+"/time/timer/current", nil, &raw); err != nil {
		return "", time.Time{}, err
	}
	if err := json.Unmarshal(raw, &current); err != nil || current == nil {
		return "", time.Time{}, nil
	}
	return current.TaskCode, current.StartedAt, nil
}

// GetTasks is the person's open tasks: every card in their name, on every
// project they are on, that is not in a finished column. `status` is kept
// for the callers that pass "active"; it is the only thing this returns.
func (c *Client) GetTasks(status string) ([]Task, error) {
	_ = status
	places, err := c.places()
	if err != nil {
		return nil, err
	}

	// What the timer is on, a workspace at a time. A failure here costs the
	// "tracking" marker, not the list.
	tracked := map[string]string{}
	var trackedLock sync.Mutex
	seen := map[string]bool{}
	workspaceIDs := []string{}
	for _, place := range places {
		if !seen[place.WorkspaceID] {
			seen[place.WorkspaceID] = true
			workspaceIDs = append(workspaceIDs, place.WorkspaceID)
		}
	}
	_ = each(len(workspaceIDs), func(i int) error {
		if code, _, err := c.tracking(workspaceIDs[i]); err == nil && code != "" {
			trackedLock.Lock()
			tracked[workspaceIDs[i]] = code
			trackedLock.Unlock()
		}
		return nil
	})

	perPlace := make([][]Task, len(places))
	if err := each(len(places), func(i int) error {
		place := places[i]
		columns, err := c.board(place, true)
		if err != nil {
			return err
		}
		for index, column := range columns {
			if column.IsDone {
				continue
			}
			for _, card := range column.Tasks {
				remember(card.Code, place)
				perPlace[i] = append(perPlace[i], Task{
					Code:     card.Code,
					Title:    card.Title,
					Project:  place.ProjectName,
					Status:   statusOf(columns, index),
					Tracking: strings.EqualFold(tracked[place.WorkspaceID], card.Code),
					URL:      c.TaskURL(place, card.Code),
				})
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	tasks := []Task{}
	for _, got := range perPlace {
		tasks = append(tasks, got...)
	}
	return tasks, nil
}

// ── projects, and making a task ─────────────────────────────────────────────

// Project is a project the person can create tasks in. ID is the workspace
// and the code together (see Place); Code is what a person reads.
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Workspace string `json:"workspace"`
}

// knownProjects is the Place behind each Project.ID that GetProjects handed
// out, so CreateTask is told where and does not have to guess.
var knownProjects = struct {
	sync.Mutex
	byID map[string]Place
}{byID: map[string]Place{}}

// GetProjects returns the projects the person is on.
func (c *Client) GetProjects() ([]Project, error) {
	places, err := c.places()
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(places))
	knownProjects.Lock()
	defer knownProjects.Unlock()
	for _, place := range places {
		knownProjects.byID[place.key()] = place
		projects = append(projects, Project{ID: place.key(), Name: place.ProjectName, Code: place.ProjectCode, Workspace: place.WorkspaceName})
	}
	return projects, nil
}

// CreateTask creates a task in the given project (a Project.ID from
// GetProjects), in the person's own name: a task made from a commit screen
// is one they are about to work on. Where it lands on the board is the
// project's to say.
func (c *Client) CreateTask(projectID string, title string) (Task, error) {
	knownProjects.Lock()
	place, ok := knownProjects.byID[projectID]
	knownProjects.Unlock()
	if !ok {
		return Task{}, errors.New("that project is not one you are on")
	}
	created := struct {
		Code string `json:"code"`
	}{}
	body := map[string]string{"title": title, "assigneeId": c.UserID}
	if err := c.call(http.MethodPost, place.path()+"/tasks", body, &created); err != nil {
		return Task{}, fmt.Errorf("creating the task: %w", err)
	}
	remember(created.Code, place)
	return Task{
		Code:    created.Code,
		Title:   title,
		Project: place.ProjectName,
		Status:  StatusTodo,
		URL:     c.TaskURL(place, created.Code),
	}, nil
}

// ── moving a task ───────────────────────────────────────────────────────────

// Column is a board column a task can be moved to. Slug is the column's id.
type Column struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Position int    `json:"position"`
}

// GetTaskColumns returns the columns of the task's project, in board order.
func (c *Client) GetTaskColumns(code string) ([]Column, error) {
	place, err := c.placeOf(code)
	if err != nil {
		return nil, err
	}
	columns, err := c.board(place, true)
	if err != nil {
		return nil, fmt.Errorf("reading the board: %w", err)
	}
	out := make([]Column, 0, len(columns))
	for index, column := range columns {
		out = append(out, Column{Slug: column.ID, Name: column.Name, Role: statusOf(columns, index), Position: column.Position})
	}
	return out, nil
}

// MoveTaskToColumn moves the task to the column with the given id. The
// board's own rules apply (a column's allowed moves, its limit a person):
// a refusal comes back in the API's words.
func (c *Client) MoveTaskToColumn(code string, columnSlug string) error {
	place, err := c.placeOf(code)
	if err != nil {
		return err
	}
	path := place.path() + "/tasks/" + url.PathEscape(strings.ToUpper(code)) + "/move"
	if err := c.call(http.MethodPost, path, map[string]string{"columnId": columnSlug}, nil); err != nil {
		return fmt.Errorf("moving the task: %w", err)
	}
	return nil
}

// SuppressAutoDone is kept for its callers and does nothing. The server
// before DeskTick moved a task to Done by itself when its pull request was
// merged, and this told it not to, once. DeskTick moves nothing by itself:
// a task goes where the person puts it.
func (c *Client) SuppressAutoDone(code string) error {
	_ = code
	return nil
}
