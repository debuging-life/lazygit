package presentation

import (
	"testing"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestDeskgitPrMark(t *testing.T) {
	scenarios := []struct {
		name     string
		pr       models.GithubPullRequest
		expected string
	}{
		{"waiting", models.GithubPullRequest{State: "OPEN"}, ""},
		{"approved", models.GithubPullRequest{State: "OPEN", Review: desktimers.ReviewApproved}, "✓"},
		{"a draft approved", models.GithubPullRequest{State: "DRAFT", Review: desktimers.ReviewApproved}, "✓"},
		{"changes asked for", models.GithubPullRequest{State: "OPEN", Review: desktimers.ReviewChangesRequested}, "✗"},
		{"a conflict outranks an approval", models.GithubPullRequest{State: "OPEN", Review: desktimers.ReviewApproved, Conflicting: true}, "!"},
		{"merged says nothing more", models.GithubPullRequest{State: "MERGED", Review: desktimers.ReviewApproved}, ""},
		{"closed says nothing more", models.GithubPullRequest{State: "CLOSED", Conflicting: true}, ""},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			assert.Equal(t, s.expected, utils.Decolorise(deskgitPrMark(&s.pr)))
		})
	}
}
