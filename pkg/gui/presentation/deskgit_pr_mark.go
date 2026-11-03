package presentation

import (
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
)

// deskgit: this file is the fork's own. An open pull request's icon is
// followed by one mark saying what it is waiting on: ! it conflicts with
// its base, ✗ changes were asked for, ✓ it is approved. A conflict comes
// first because nothing merges until it is resolved, approved or not.
func deskgitPrMark(pr *models.GithubPullRequest) string {
	if pr.State != "OPEN" && pr.State != "DRAFT" {
		return ""
	}
	switch {
	case pr.Conflicting:
		return style.FgRed.Sprint("!")
	case pr.Review == desktimers.ReviewChangesRequested:
		return style.FgRed.Sprint("✗")
	case pr.Review == desktimers.ReviewApproved:
		return style.FgGreen.Sprint("✓")
	default:
		return ""
	}
}
