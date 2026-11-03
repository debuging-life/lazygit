package desktimers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadPullRequestReadsItAsGitHubHasIt(t *testing.T) {
	github := newFakeGitHub(t)

	pr, err := github.repo().LoadPullRequest(7)
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.Equal(t, "PR_7", pr.NodeID)
	assert.Equal(t, "https://github.com/acme/clock/pull/7", pr.URL)
	assert.Equal(t, PullRequestDraft{
		Title: "CLK-12/Show the clock", Body: "Task: x", Head: "feature/CLK-12-clock", Base: "main", Draft: true,
		Reviewers: []string{"zed"}, TeamReviewers: []string{"core"},
		Assignees: []string{"ada"}, Labels: []string{"bug"}, ProjectIDs: []string{"P1"},
	}, pr.Draft)
}

func TestLoadPullRequestWithoutTheProjectScope(t *testing.T) {
	github := newFakeGitHub(t)
	github.noProjectScope = true

	pr, err := github.repo().LoadPullRequest(7)

	assert.NoError(t, err)
	assert.Empty(t, pr.Draft.ProjectIDs)
}

func TestUpdatePullRequestSendsNothingWhenNothingChanged(t *testing.T) {
	github := newFakeGitHub(t)
	pr, _ := github.repo().LoadPullRequest(7)
	github.requests = nil

	saved, err := github.repo().UpdatePullRequest(pr, pr.Draft)

	assert.NoError(t, err)
	assert.Empty(t, saved.Warnings)
	assert.Empty(t, github.requests)
}

func TestUpdatePullRequestSendsOnlyWhatChanged(t *testing.T) {
	github := newFakeGitHub(t)
	pr, _ := github.repo().LoadPullRequest(7)
	github.requests = nil

	edited := pr.Draft
	edited.Title = "CLK-12/Show the clock face"
	edited.Draft = false
	edited.Reviewers = []string{"Bea"}
	edited.Labels = []string{"bug", "ui"}
	edited.ProjectIDs = []string{"P3"}

	saved, err := github.repo().UpdatePullRequest(pr, edited)

	assert.NoError(t, err)
	assert.Empty(t, saved.Warnings)
	assert.Equal(t, []string{
		"PATCH /repos/acme/clock/pulls/7",
		"POST /graphql",
		"DELETE /repos/acme/clock/pulls/7/requested_reviewers",
		"POST /repos/acme/clock/pulls/7/requested_reviewers",
		"PATCH /repos/acme/clock/issues/7",
		"POST /graphql",
		"POST /graphql",
	}, github.requests)
	assert.Equal(t, map[string]any{"title": "CLK-12/Show the clock face"}, github.bodies["PATCH /repos/acme/clock/pulls/7"])
	assert.Equal(t, map[string]any{"reviewers": []any{"zed"}, "team_reviewers": []any{}},
		github.bodies["DELETE /repos/acme/clock/pulls/7/requested_reviewers"])
	assert.Equal(t, map[string]any{"reviewers": []any{"Bea"}, "team_reviewers": []any{}},
		github.bodies["POST /repos/acme/clock/pulls/7/requested_reviewers"])
	// Labels are set whole; the assignees, unchanged, are not sent.
	assert.Equal(t, map[string]any{"labels": []any{"bug", "ui"}}, github.bodies["PATCH /repos/acme/clock/issues/7"])
}

func TestUpdatePullRequestMarksItReadyOrADraft(t *testing.T) {
	for _, draft := range []bool{false, true} {
		github := newFakeGitHub(t)
		pr, _ := github.repo().LoadPullRequest(7)
		pr.Draft.Draft = !draft
		var queries []string
		github.requests = nil

		edited := pr.Draft
		edited.Draft = draft
		saved, err := github.repo().UpdatePullRequest(pr, edited)
		assert.NoError(t, err)
		assert.Equal(t, draft, saved.Draft)
		for _, key := range github.requests {
			if key == "POST /graphql" {
				queries = append(queries, github.bodies[key]["query"].(string))
			}
		}
		if assert.Len(t, queries, 1) {
			if draft {
				assert.Contains(t, queries[0], "convertPullRequestToDraft")
			} else {
				assert.Contains(t, queries[0], "markPullRequestReadyForReview")
			}
		}
	}
}

func TestUpdatePullRequestTakesItOffAProjectByItsItem(t *testing.T) {
	github := newFakeGitHub(t)
	pr, _ := github.repo().LoadPullRequest(7)

	edited := pr.Draft
	edited.ProjectIDs = nil
	_, err := github.repo().UpdatePullRequest(pr, edited)

	assert.NoError(t, err)
	variables := github.bodies["POST /graphql"]["variables"].(map[string]any)
	assert.Equal(t, map[string]any{"project": "P1", "item": "ITEM_P1"}, variables)
}

func TestUpdatePullRequestStopsWhenTheTitleIsRefused(t *testing.T) {
	github := newFakeGitHub(t)
	pr, _ := github.repo().LoadPullRequest(7)
	github.refuseTitle = true
	github.requests = nil

	edited := pr.Draft
	edited.Title = ""
	edited.Labels = nil
	_, err := github.repo().UpdatePullRequest(pr, edited)

	assert.EqualError(t, err, "Validation Failed")
	assert.Equal(t, []string{"PATCH /repos/acme/clock/pulls/7"}, github.requests)
}

func TestUpdatePullRequestKeepsGoingWhenAReviewerIsRefused(t *testing.T) {
	github := newFakeGitHub(t)
	github.refuseReviewer = true
	pr, _ := github.repo().LoadPullRequest(7)

	edited := pr.Draft
	edited.Reviewers = []string{"zed", "Bea"}
	edited.Labels = nil
	saved, err := github.repo().UpdatePullRequest(pr, edited)

	assert.NoError(t, err)
	assert.Equal(t, []string{"reviewers: Reviews may only be requested from collaborators."}, saved.Warnings)
	assert.Equal(t, map[string]any{"labels": []any{}}, github.bodies["PATCH /repos/acme/clock/issues/7"])
}

func TestUpdatePullRequestSaysItIsStillADraftWhenMarkingItReadyIsRefused(t *testing.T) {
	github := newFakeGitHub(t)
	pr, _ := github.repo().LoadPullRequest(7)
	github.noProjectScope = true // every GraphQL call is refused

	edited := pr.Draft
	edited.Draft = false
	saved, err := github.repo().UpdatePullRequest(pr, edited)

	assert.NoError(t, err)
	assert.True(t, saved.Draft)
	assert.Len(t, saved.Warnings, 1)
}
