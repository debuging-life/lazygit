package desktimers

// Where an open pull request stands with its reviewers, as the branches
// panel shows it beside the branch.
const (
	ReviewApproved         = "APPROVED"
	ReviewChangesRequested = "CHANGES_REQUESTED"
	ReviewWaiting          = "REVIEW_REQUIRED"
)

// PullRequestReview reads GitHub's review state. GitHub's own verdict
// (reviewDecision) is there only when the base branch requires reviews;
// otherwise each reviewer's latest approval or request for changes is what
// there is, and a request for changes outweighs any approval, as GitHub
// weighs them.
func PullRequestReview(decision string, latestOpinions []string) string {
	if decision != "" {
		return decision
	}
	review := ""
	for _, state := range latestOpinions {
		switch state {
		case ReviewChangesRequested:
			return ReviewChangesRequested
		case ReviewApproved:
			review = ReviewApproved
		}
	}
	return review
}
