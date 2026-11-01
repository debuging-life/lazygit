package desktimers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ── creating a pull request with everything on it ───────────────────────────
// deskgit's pull request form: title, description, base, reviewers,
// assignees, labels, projects and draft, sent to GitHub as the person, with
// the token their gh sign-in already has. Only GitHub's own API is asked;
// DeskTick has no part in it.

// githubTimeout bounds every GitHub request the form makes, so a slow GitHub
// shows as an error rather than a spinner that never stops.
const githubTimeout = 15 * time.Second

// githubListPages bounds how far a list (assignable people, labels) is
// followed: 500 of each is more than a picker is any use for.
const githubListPages = 5

// ErrProjectsNeedScope says the token cannot see GitHub Projects: gh signs
// in without the project scope unless asked for it.
var ErrProjectsNeedScope = errors.New("GitHub Projects need the project scope: run `gh auth refresh -s project`")

// GitHubRepo is one repository on one GitHub host, asked as one person.
type GitHubRepo struct {
	// Host is the web host: github.com, or a GitHub Enterprise Server's.
	Host  string
	Owner string
	Name  string
	Token string
	// apiBase and graphQLURL override the host's addresses in tests.
	apiBase    string
	graphQLURL string
}

func (r GitHubRepo) restBase() string {
	if r.apiBase != "" {
		return r.apiBase
	}
	if r.Host == "" || r.Host == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + r.Host + "/api/v3"
}

func (r GitHubRepo) graphQLEndpoint() string {
	if r.graphQLURL != "" {
		return r.graphQLURL
	}
	if r.Host == "" || r.Host == "github.com" {
		return "https://api.github.com/graphql"
	}
	return "https://" + r.Host + "/api/graphql"
}

// GitHubUser is somebody who can be asked to review or be assigned.
type GitHubUser struct {
	Login string `json:"login"`
}

// GitHubTeam is an organisation's team that can be asked to review.
type GitHubTeam struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// GitHubLabel is one of the repository's labels.
type GitHubLabel struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GitHubProject is a GitHub Project (the current kind, "Projects v2").
type GitHubProject struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Number int    `json:"number"`
	Closed bool   `json:"closed"`
}

// PullRequestChoices is what the form offers: who and what can go on the
// pull request. ProjectsErr is kept apart so the rest of the form works
// without them.
type PullRequestChoices struct {
	DefaultBranch string
	Viewer        string
	Users         []GitHubUser
	Teams         []GitHubTeam
	Labels        []GitHubLabel
	Projects      []GitHubProject
	ProjectsErr   error
}

// ReviewerCandidates are the people who can be asked to review: everybody
// assignable but the author, whom GitHub refuses to ask.
func (c *PullRequestChoices) ReviewerCandidates() []GitHubUser {
	candidates := []GitHubUser{}
	for _, user := range c.Users {
		if !strings.EqualFold(user.Login, c.Viewer) {
			candidates = append(candidates, user)
		}
	}
	return candidates
}

// GitHubError is an answer GitHub gave other than the one asked for.
type GitHubError struct {
	Status  int
	Message string
}

func (e *GitHubError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("GitHub answered %d", e.Status)
}

func (r GitHubRepo) do(method string, url string, body any, out any) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+r.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: githubTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp, githubError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp, fmt.Errorf("reading GitHub's answer: %w", err)
		}
	}
	return resp, nil
}

// githubError reads GitHub's error body: a message, and for a 422 the
// detail of what was refused, which is the part worth showing.
func githubError(status int, data []byte) error {
	var parsed struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(data, &parsed)
	message := parsed.Message
	// The first detail that says something is the one shown.
	for _, e := range parsed.Errors {
		if e.Message != "" {
			message = e.Message
			break
		}
		if e.Field != "" && e.Code != "" {
			message = fmt.Sprintf("%s: %s %s", parsed.Message, e.Field, e.Code)
			break
		}
	}
	return &GitHubError{Status: status, Message: message}
}

var nextLinkRegex = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// list follows a REST list across its pages, githubListPages at most.
func list[T any](r GitHubRepo, path string) ([]T, error) {
	all := []T{}
	url := r.restBase() + path
	for page := 0; page < githubListPages && url != ""; page++ {
		var items []T
		resp, err := r.do(http.MethodGet, url, nil, &items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		url = ""
		if match := nextLinkRegex.FindStringSubmatch(resp.Header.Get("Link")); match != nil {
			url = match[1]
		}
	}
	return all, nil
}

func (r GitHubRepo) repoPath() string {
	return "/repos/" + r.Owner + "/" + r.Name
}

// LoadPullRequestChoices asks GitHub, all at once, for what the form offers.
// The repository's default branch and its people must be there; teams are
// an organisation's only and labels may be none, so either failing leaves
// that list empty, and projects carry their own error.
func (r GitHubRepo) LoadPullRequestChoices() (*PullRequestChoices, error) {
	choices := &PullRequestChoices{}
	var wg sync.WaitGroup
	var repoErr, usersErr, viewerErr error

	wg.Add(6)
	go func() {
		defer wg.Done()
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		_, repoErr = r.do(http.MethodGet, r.restBase()+r.repoPath(), nil, &repo)
		choices.DefaultBranch = repo.DefaultBranch
	}()
	go func() {
		defer wg.Done()
		var viewer GitHubUser
		_, viewerErr = r.do(http.MethodGet, r.restBase()+"/user", nil, &viewer)
		choices.Viewer = viewer.Login
	}()
	go func() {
		defer wg.Done()
		choices.Users, usersErr = list[GitHubUser](r, r.repoPath()+"/assignees?per_page=100")
	}()
	go func() {
		defer wg.Done()
		choices.Teams, _ = list[GitHubTeam](r, r.repoPath()+"/teams?per_page=100")
	}()
	go func() {
		defer wg.Done()
		choices.Labels, _ = list[GitHubLabel](r, r.repoPath()+"/labels?per_page=100")
	}()
	go func() {
		defer wg.Done()
		choices.Projects, choices.ProjectsErr = r.projects()
	}()
	wg.Wait()

	if repoErr != nil {
		return nil, repoErr
	}
	if usersErr != nil {
		return nil, usersErr
	}
	// Who is asking decides who may review: without it the author would
	// be offered, and GitHub refuses every reviewer of a request naming
	// them.
	if viewerErr != nil {
		return nil, viewerErr
	}
	sort.Slice(choices.Users, func(i, j int) bool {
		return strings.ToLower(choices.Users[i].Login) < strings.ToLower(choices.Users[j].Login)
	})
	return choices, nil
}

type graphQLError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (r GitHubRepo) graphQL(query string, variables map[string]any, out any) error {
	var answer struct {
		Data   json.RawMessage `json:"data"`
		Errors []graphQLError  `json:"errors"`
	}
	if _, err := r.do(http.MethodPost, r.graphQLEndpoint(), map[string]any{"query": query, "variables": variables}, &answer); err != nil {
		return err
	}
	for _, e := range answer.Errors {
		if e.Type == "INSUFFICIENT_SCOPES" {
			return ErrProjectsNeedScope
		}
	}
	if len(answer.Errors) > 0 {
		return errors.New(answer.Errors[0].Message)
	}
	if out != nil && len(answer.Data) > 0 {
		return json.Unmarshal(answer.Data, out)
	}
	return nil
}

const projectsQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    projectsV2(first: 50) { nodes { id title number closed } }
    owner {
      ... on Organization { projectsV2(first: 50) { nodes { id title number closed } } }
      ... on User { projectsV2(first: 50) { nodes { id title number closed } } }
    }
  }
}`

// projects are the open projects of the repository and of its owner, each
// once.
func (r GitHubRepo) projects() ([]GitHubProject, error) {
	type nodes struct {
		Nodes []GitHubProject `json:"nodes"`
	}
	var data struct {
		Repository struct {
			ProjectsV2 nodes `json:"projectsV2"`
			Owner      struct {
				ProjectsV2 nodes `json:"projectsV2"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := r.graphQL(projectsQuery, map[string]any{"owner": r.Owner, "name": r.Name}, &data); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	projects := []GitHubProject{}
	for _, p := range append(data.Repository.ProjectsV2.Nodes, data.Repository.Owner.ProjectsV2.Nodes...) {
		if p.Closed || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		projects = append(projects, p)
	}
	return projects, nil
}

// PullRequestDraft is the form as filled in.
type PullRequestDraft struct {
	Title string
	Body  string
	// Head is the branch on GitHub the changes are on; Base is where they go.
	Head  string
	Base  string
	Draft bool
	// Reviewers are logins; TeamReviewers are team slugs.
	Reviewers     []string
	TeamReviewers []string
	Assignees     []string
	Labels        []string
	// ProjectIDs are the node ids of GitHub Projects.
	ProjectIDs []string
}

// CreatedPullRequest is the pull request made. Warnings say what of the
// rest GitHub refused: the pull request is there either way, and making it
// again would make a second one.
type CreatedPullRequest struct {
	Number   int
	URL      string
	Warnings []string
}

// CreatePullRequest makes the pull request, then puts on it the reviewers,
// the assignees and labels, and the projects, each a request of its own as
// GitHub has them.
func (r GitHubRepo) CreatePullRequest(draft PullRequestDraft) (*CreatedPullRequest, error) {
	if draft.Base == "" {
		return nil, errors.New("a pull request needs a base branch")
	}
	var made struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		NodeID  string `json:"node_id"`
	}
	_, err := r.do(http.MethodPost, r.restBase()+r.repoPath()+"/pulls", map[string]any{
		"title": draft.Title,
		"body":  draft.Body,
		"head":  draft.Head,
		"base":  draft.Base,
		"draft": draft.Draft,
	}, &made)
	if err != nil {
		return nil, err
	}
	created := &CreatedPullRequest{Number: made.Number, URL: made.HTMLURL}
	warn := func(what string, err error) {
		created.Warnings = append(created.Warnings, what+": "+err.Error())
	}

	if len(draft.Reviewers) > 0 || len(draft.TeamReviewers) > 0 {
		_, err := r.do(http.MethodPost, fmt.Sprintf("%s%s/pulls/%d/requested_reviewers", r.restBase(), r.repoPath(), made.Number),
			map[string]any{"reviewers": nonNil(draft.Reviewers), "team_reviewers": nonNil(draft.TeamReviewers)}, nil)
		if err != nil {
			warn("reviewers", err)
		}
	}
	if len(draft.Assignees) > 0 || len(draft.Labels) > 0 {
		patch := map[string]any{}
		if len(draft.Assignees) > 0 {
			patch["assignees"] = draft.Assignees
		}
		if len(draft.Labels) > 0 {
			patch["labels"] = draft.Labels
		}
		if _, err := r.do(http.MethodPatch, fmt.Sprintf("%s%s/issues/%d", r.restBase(), r.repoPath(), made.Number), patch, nil); err != nil {
			warn("assignees and labels", err)
		}
	}
	for _, projectID := range draft.ProjectIDs {
		err := r.graphQL(`mutation($project: ID!, $content: ID!) {
  addProjectV2ItemById(input: {projectId: $project, contentId: $content}) { item { id } }
}`, map[string]any{"project": projectID, "content": made.NodeID}, nil)
		if err != nil {
			warn("project", err)
		}
	}
	return created, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
