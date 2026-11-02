package desktimers

import "net/http"

// ── pull requests on task cards ─────────────────────────────────────────────
// A pull request deskgit makes, edits or merges is told to DeskTick at once
// (POST /api/github/pulls), so the task card shows it without waiting for
// GitHub's webhook. GitHub's delivery is the truth and replaces what this
// said; this never moves a task on the board, the delivery does.

// Pull request states as the route takes them.
const (
	PullOpen   = "open"
	PullClosed = "closed"
	PullMerged = "merged"
)

// PullRequestNotice is a pull request as the route takes it.
type PullRequestNotice struct {
	// Repo is the origin remote's "owner/name".
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Head   string `json:"head"`
	Base   string `json:"base"`
	// TaskCode is the task picked in deskgit, nil when none; the API reads
	// codes from the title and the branch as well.
	TaskCode *string `json:"taskCode"`
}

// SendPullRequest tells DeskTick of a pull request, and answers which
// tasks it was put on: none when no code it knows is in it, or no project
// of the person's links the repository.
func (c *Client) SendPullRequest(notice PullRequestNotice) ([]string, error) {
	var answer struct {
		TaskCodes []string `json:"taskCodes"`
	}
	if err := c.call(http.MethodPost, "/github/pulls", notice, &answer); err != nil {
		return nil, err
	}
	return answer.TaskCodes, nil
}
