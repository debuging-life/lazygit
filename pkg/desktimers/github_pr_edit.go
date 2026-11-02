package desktimers

import (
	"fmt"
	"net/http"
	"slices"
)

// ── editing a pull request that is open ─────────────────────────────────────
// The same form edits a pull request already on GitHub: it is read as it
// is, and saving sends only what changed, each part a request of its own
// as GitHub has them.

// OpenPullRequest is a pull request as GitHub has it, to be edited.
type OpenPullRequest struct {
	Number int
	URL    string
	NodeID string
	// Draft is the form as GitHub has it; its Draft says whether the pull
	// request is still a draft.
	Draft PullRequestDraft
	// projectItems is the pull request's item in each project it is on,
	// by project id: taking it off a project names the item.
	projectItems map[string]string
}

const pullRequestProjectsQuery = `query($id: ID!) {
  node(id: $id) {
    ... on PullRequest { projectItems(first: 50) { nodes { id project { id } } } }
  }
}`

// LoadPullRequest reads an open pull request. The projects it is on need
// the project scope; without it they are left out, which the form already
// says, and nothing is taken off a project on save.
func (r GitHubRepo) LoadPullRequest(number int) (*OpenPullRequest, error) {
	type login struct {
		Login string `json:"login"`
	}
	var got struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		Draft   bool   `json:"draft"`
		NodeID  string `json:"node_id"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		RequestedReviewers []login `json:"requested_reviewers"`
		RequestedTeams     []struct {
			Slug string `json:"slug"`
		} `json:"requested_teams"`
		Assignees []login `json:"assignees"`
		Labels    []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if _, err := r.do(http.MethodGet, fmt.Sprintf("%s%s/pulls/%d", r.restBase(), r.repoPath(), number), nil, &got); err != nil {
		return nil, err
	}
	pr := &OpenPullRequest{
		Number: number, URL: got.HTMLURL, NodeID: got.NodeID,
		Draft: PullRequestDraft{
			Title: got.Title, Body: got.Body, Head: got.Head.Ref, Base: got.Base.Ref, Draft: got.Draft,
		},
		projectItems: map[string]string{},
	}
	for _, user := range got.RequestedReviewers {
		pr.Draft.Reviewers = append(pr.Draft.Reviewers, user.Login)
	}
	for _, team := range got.RequestedTeams {
		pr.Draft.TeamReviewers = append(pr.Draft.TeamReviewers, team.Slug)
	}
	for _, user := range got.Assignees {
		pr.Draft.Assignees = append(pr.Draft.Assignees, user.Login)
	}
	for _, label := range got.Labels {
		pr.Draft.Labels = append(pr.Draft.Labels, label.Name)
	}

	var items struct {
		Node struct {
			ProjectItems struct {
				Nodes []struct {
					ID      string `json:"id"`
					Project struct {
						ID string `json:"id"`
					} `json:"project"`
				} `json:"nodes"`
			} `json:"projectItems"`
		} `json:"node"`
	}
	if err := r.graphQL(pullRequestProjectsQuery, map[string]any{"id": got.NodeID}, &items); err == nil {
		for _, item := range items.Node.ProjectItems.Nodes {
			pr.Draft.ProjectIDs = append(pr.Draft.ProjectIDs, item.Project.ID)
			pr.projectItems[item.Project.ID] = item.ID
		}
	}
	return pr, nil
}

// UpdatePullRequest sends what changed from pr to edited. The title,
// description and base go first and alone: if GitHub refuses them nothing
// else is sent. What it refuses of the rest comes back as warnings, the
// rest having been saved.
func (r GitHubRepo) UpdatePullRequest(pr *OpenPullRequest, edited PullRequestDraft) ([]string, error) {
	before := pr.Draft
	base := fmt.Sprintf("%s%s", r.restBase(), r.repoPath())

	patch := map[string]any{}
	if edited.Title != before.Title {
		patch["title"] = edited.Title
	}
	if edited.Body != before.Body {
		patch["body"] = edited.Body
	}
	if edited.Base != before.Base {
		patch["base"] = edited.Base
	}
	if len(patch) > 0 {
		if _, err := r.do(http.MethodPatch, fmt.Sprintf("%s/pulls/%d", base, pr.Number), patch, nil); err != nil {
			return nil, err
		}
	}

	warnings := []string{}
	warn := func(what string, err error) {
		if err != nil {
			warnings = append(warnings, what+": "+err.Error())
		}
	}

	if edited.Draft != before.Draft {
		mutation := `mutation($id: ID!) { markPullRequestReadyForReview(input: {pullRequestId: $id}) { clientMutationId } }`
		if edited.Draft {
			mutation = `mutation($id: ID!) { convertPullRequestToDraft(input: {pullRequestId: $id}) { clientMutationId } }`
		}
		warn("draft", r.graphQL(mutation, map[string]any{"id": pr.NodeID}, nil))
	}

	reviewers := fmt.Sprintf("%s/pulls/%d/requested_reviewers", base, pr.Number)
	addUsers, dropUsers := changed(before.Reviewers, edited.Reviewers)
	addTeams, dropTeams := changed(before.TeamReviewers, edited.TeamReviewers)
	if len(dropUsers) > 0 || len(dropTeams) > 0 {
		_, err := r.do(http.MethodDelete, reviewers, map[string]any{"reviewers": nonNil(dropUsers), "team_reviewers": nonNil(dropTeams)}, nil)
		warn("reviewers", err)
	}
	if len(addUsers) > 0 || len(addTeams) > 0 {
		_, err := r.do(http.MethodPost, reviewers, map[string]any{"reviewers": nonNil(addUsers), "team_reviewers": nonNil(addTeams)}, nil)
		warn("reviewers", err)
	}

	// An issue's assignees and labels are each set whole.
	issue := map[string]any{}
	if !sameSet(before.Assignees, edited.Assignees) {
		issue["assignees"] = nonNil(edited.Assignees)
	}
	if !sameSet(before.Labels, edited.Labels) {
		issue["labels"] = nonNil(edited.Labels)
	}
	if len(issue) > 0 {
		_, err := r.do(http.MethodPatch, fmt.Sprintf("%s/issues/%d", base, pr.Number), issue, nil)
		warn("assignees and labels", err)
	}

	addProjects, dropProjects := changed(before.ProjectIDs, edited.ProjectIDs)
	for _, projectID := range addProjects {
		warn("project", r.graphQL(`mutation($project: ID!, $content: ID!) {
  addProjectV2ItemById(input: {projectId: $project, contentId: $content}) { item { id } }
}`, map[string]any{"project": projectID, "content": pr.NodeID}, nil))
	}
	for _, projectID := range dropProjects {
		warn("project", r.graphQL(`mutation($project: ID!, $item: ID!) {
  deleteProjectV2Item(input: {projectId: $project, itemId: $item}) { deletedItemId }
}`, map[string]any{"project": projectID, "item": pr.projectItems[projectID]}, nil))
	}
	return warnings, nil
}

// changed says what is in after and not before, and what is in before
// and not after.
func changed(before []string, after []string) (added []string, removed []string) {
	for _, value := range after {
		if !slices.Contains(before, value) {
			added = append(added, value)
		}
	}
	for _, value := range before {
		if !slices.Contains(after, value) {
			removed = append(removed, value)
		}
	}
	return added, removed
}

func sameSet(a []string, b []string) bool {
	added, removed := changed(a, b)
	return len(added) == 0 && len(removed) == 0
}
