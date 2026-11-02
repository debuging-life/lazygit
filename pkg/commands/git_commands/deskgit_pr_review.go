package git_commands

import (
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

// deskgit: this file is the fork's own. The branches panel shows where an
// open pull request stands with its reviewers and whether it conflicts
// with its base; these are the fields of upstream's query that say so.
type deskgitReviewFields struct {
	ReviewDecision           string `json:"reviewDecision"`
	Mergeable                string `json:"mergeable"`
	LatestOpinionatedReviews struct {
		Nodes []struct {
			State string `json:"state"`
		} `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
}

func (f deskgitReviewFields) applyTo(pr *models.GithubPullRequest) {
	opinions := make([]string, 0, len(f.LatestOpinionatedReviews.Nodes))
	for _, review := range f.LatestOpinionatedReviews.Nodes {
		opinions = append(opinions, review.State)
	}
	pr.Review = desktimers.PullRequestReview(f.ReviewDecision, opinions)
	pr.Conflicting = f.Mergeable == "CONFLICTING"
}
