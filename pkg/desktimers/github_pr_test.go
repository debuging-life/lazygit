package desktimers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeGitHub is a stand-in for GitHub's REST and GraphQL APIs, answering
// what the pull request form asks.
type fakeGitHub struct {
	t      *testing.T
	server *httptest.Server

	mutex sync.Mutex
	// requests are "METHOD path" with each request's body, in order.
	requests []string
	bodies   map[string]map[string]any

	noProjectScope bool
	refuseReviewer bool
	noViewer       bool
	noRepo         bool
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{t: t, bodies: map[string]map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token gho_secret", r.Header.Get("Authorization"))
		var body map[string]any
		if data, _ := io.ReadAll(r.Body); len(data) > 0 {
			_ = json.Unmarshal(data, &body)
		}
		key := r.Method + " " + r.URL.Path
		if r.URL.RawQuery != "" && r.Method == http.MethodGet {
			key += "?" + r.URL.RawQuery
		}
		fake.mutex.Lock()
		fake.requests = append(fake.requests, key)
		fake.bodies[key] = body
		fake.mutex.Unlock()
		fake.answer(w, key, body)
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGitHub) repo() GitHubRepo {
	return GitHubRepo{
		Owner: "acme", Name: "clock", Token: "gho_secret",
		apiBase: f.server.URL, graphQLURL: f.server.URL + "/graphql",
	}
}

func (f *fakeGitHub) answer(w http.ResponseWriter, key string, body map[string]any) {
	write := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	// What a token that cannot see the repository is told.
	case f.noRepo && strings.HasPrefix(key, "GET /repos/acme/clock"):
		write(404, map[string]any{"message": "Not Found"})
	case key == "GET /repos/acme/clock":
		write(200, map[string]any{"default_branch": "main"})
	case key == "GET /user":
		if f.noViewer {
			write(401, map[string]any{"message": "Requires authentication"})
			return
		}
		write(200, map[string]any{"login": "ada"})
	case key == "GET /repos/acme/clock/assignees?per_page=100":
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/clock/assignees?per_page=100&page=2>; rel="next"`, f.server.URL))
		write(200, []map[string]any{{"login": "zed"}, {"login": "ada"}})
	case key == "GET /repos/acme/clock/assignees?page=2&per_page=100", key == "GET /repos/acme/clock/assignees?per_page=100&page=2":
		write(200, []map[string]any{{"login": "Bea"}})
	case strings.HasPrefix(key, "GET /repos/acme/clock/teams"):
		write(404, map[string]any{"message": "Not Found"})
	case strings.HasPrefix(key, "GET /repos/acme/clock/labels"):
		write(200, []map[string]any{{"name": "bug"}, {"name": "ui", "description": "Looks"}})
	case key == "POST /graphql":
		query, _ := body["query"].(string)
		if f.noProjectScope {
			write(200, map[string]any{"errors": []map[string]any{{
				"type": "INSUFFICIENT_SCOPES", "message": "Your token has not been granted the required scopes",
			}}})
			return
		}
		if strings.Contains(query, "addProjectV2ItemById") {
			write(200, map[string]any{"data": map[string]any{"addProjectV2ItemById": map[string]any{"item": map[string]any{"id": "I1"}}}})
			return
		}
		write(200, map[string]any{"data": map[string]any{"repository": map[string]any{
			"projectsV2": map[string]any{"nodes": []map[string]any{
				{"id": "P1", "title": "Roadmap", "number": 1},
				{"id": "P2", "title": "Old", "number": 2, "closed": true},
			}},
			"owner": map[string]any{"projectsV2": map[string]any{"nodes": []map[string]any{
				{"id": "P1", "title": "Roadmap", "number": 1},
				{"id": "P3", "title": "Bugs", "number": 3},
			}}},
		}}})
	case key == "POST /repos/acme/clock/pulls":
		write(201, map[string]any{"number": 7, "html_url": "https://github.com/acme/clock/pull/7", "node_id": "PR_7"})
	case key == "POST /repos/acme/clock/pulls/7/requested_reviewers":
		if f.refuseReviewer {
			write(422, map[string]any{"message": "Reviews may only be requested from collaborators."})
			return
		}
		write(201, map[string]any{})
	case key == "PATCH /repos/acme/clock/issues/7":
		write(200, map[string]any{})
	default:
		f.t.Errorf("unexpected request %s", key)
		write(404, map[string]any{"message": "Not Found"})
	}
}

func TestLoadPullRequestChoices(t *testing.T) {
	github := newFakeGitHub(t)

	choices, err := github.repo().LoadPullRequestChoices()
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.Equal(t, "main", choices.DefaultBranch)
	assert.Equal(t, "ada", choices.Viewer)
	// Every page, in name order whatever the case.
	assert.Equal(t, []GitHubUser{{"ada"}, {"Bea"}, {"zed"}}, choices.Users)
	// A repository of a person, not an organisation, has no teams.
	assert.Empty(t, choices.Teams)
	assert.Equal(t, []GitHubLabel{{Name: "bug"}, {Name: "ui", Description: "Looks"}}, choices.Labels)
	// Open ones, each once, the repository's and its owner's.
	assert.NoError(t, choices.ProjectsErr)
	assert.Equal(t, []GitHubProject{{ID: "P1", Title: "Roadmap", Number: 1}, {ID: "P3", Title: "Bugs", Number: 3}}, choices.Projects)
}

func TestLoadPullRequestChoicesWithoutTheProjectScope(t *testing.T) {
	github := newFakeGitHub(t)
	github.noProjectScope = true

	choices, err := github.repo().LoadPullRequestChoices()

	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.ErrorIs(t, choices.ProjectsErr, ErrProjectsNeedScope)
	assert.Len(t, choices.Users, 3)
}

func TestCreatePullRequestPutsEverythingOnIt(t *testing.T) {
	github := newFakeGitHub(t)

	created, err := github.repo().CreatePullRequest(PullRequestDraft{
		Title: "CLK-12/Show the clock face", Body: "Why", Head: "CLK-12/face", Base: "main", Draft: true,
		Reviewers: []string{"bea"}, TeamReviewers: []string{"web"},
		Assignees: []string{"ada"}, Labels: []string{"ui"},
		ProjectIDs: []string{"P1", "P3"},
	})

	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Equal(t, 7, created.Number)
	assert.Equal(t, "https://github.com/acme/clock/pull/7", created.URL)
	assert.Empty(t, created.Warnings)
	assert.Equal(t, []string{
		"POST /repos/acme/clock/pulls",
		"POST /repos/acme/clock/pulls/7/requested_reviewers",
		"PATCH /repos/acme/clock/issues/7",
		"POST /graphql",
		"POST /graphql",
	}, github.requests)
	assert.Equal(t, map[string]any{
		"title": "CLK-12/Show the clock face", "body": "Why", "head": "CLK-12/face", "base": "main", "draft": true,
	}, github.bodies["POST /repos/acme/clock/pulls"])
	assert.Equal(t, map[string]any{"reviewers": []any{"bea"}, "team_reviewers": []any{"web"}},
		github.bodies["POST /repos/acme/clock/pulls/7/requested_reviewers"])
	assert.Equal(t, map[string]any{"assignees": []any{"ada"}, "labels": []any{"ui"}},
		github.bodies["PATCH /repos/acme/clock/issues/7"])
}

func TestCreatePullRequestAsksOnlyWhatWasFilledIn(t *testing.T) {
	github := newFakeGitHub(t)

	_, err := github.repo().CreatePullRequest(PullRequestDraft{Title: "t", Head: "h", Base: "main"})

	assert.NoError(t, err)
	assert.Equal(t, []string{"POST /repos/acme/clock/pulls"}, github.requests)
}

func TestCreatePullRequestKeepsItWhenAReviewerIsRefused(t *testing.T) {
	github := newFakeGitHub(t)
	github.refuseReviewer = true

	created, err := github.repo().CreatePullRequest(PullRequestDraft{
		Title: "t", Head: "h", Base: "main", Reviewers: []string{"stranger"}, Labels: []string{"ui"},
	})

	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Equal(t, 7, created.Number)
	assert.Equal(t, []string{"reviewers: Reviews may only be requested from collaborators."}, created.Warnings)
	// The rest still goes on.
	assert.Contains(t, github.requests, "PATCH /repos/acme/clock/issues/7")
}

func TestCreatePullRequestNeedsABase(t *testing.T) {
	github := newFakeGitHub(t)

	_, err := github.repo().CreatePullRequest(PullRequestDraft{Title: "t", Head: "h"})

	assert.Error(t, err)
	assert.Empty(t, github.requests)
}

func TestGitHubErrorShowsWhatWasRefused(t *testing.T) {
	err := githubError(422, []byte(`{"message":"Validation Failed","errors":[{"resource":"PullRequest","code":"custom","message":"No commits between main and face"}]}`))
	assert.EqualError(t, err, "No commits between main and face")

	err = githubError(422, []byte(`{"message":"Validation Failed","errors":[{"resource":"PullRequest","field":"base","code":"invalid"}]}`))
	assert.EqualError(t, err, "Validation Failed: base invalid")

	err = githubError(422, []byte(`{"message":"Validation Failed","errors":[{"message":"No commits between main and face"},{"field":"base","code":"invalid"}]}`))
	assert.EqualError(t, err, "No commits between main and face")

	err = githubError(502, []byte(`<html>`))
	assert.EqualError(t, err, "GitHub answered 502")
}

func TestReviewerCandidatesLeaveOutTheAuthor(t *testing.T) {
	choices := &PullRequestChoices{Viewer: "Ada", Users: []GitHubUser{{"ada"}, {"bea"}}}
	assert.Equal(t, []GitHubUser{{"bea"}}, choices.ReviewerCandidates())
}

func TestLoadPullRequestChoicesNeedsToKnowWhoIsAsking(t *testing.T) {
	github := newFakeGitHub(t)
	github.noViewer = true

	_, err := github.repo().LoadPullRequestChoices()

	assert.Error(t, err)
}

func TestLoadPullRequestChoicesForARepositoryTheAccountCannotSee(t *testing.T) {
	github := newFakeGitHub(t)
	github.noRepo = true

	_, err := github.repo().LoadPullRequestChoices()

	/* EXPECTED:
	assert.EqualError(t, err, "GitHub can't find acme/clock as ada")
	ACTUAL: */
	assert.EqualError(t, err, "Not Found")
}
