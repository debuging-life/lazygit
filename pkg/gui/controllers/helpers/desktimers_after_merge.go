package helpers

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

// ── after a pull request is merged ──────────────────────────────────────────
// Merging from deskgit leaves the branch behind, here and on the remote,
// and usually the person on it. TidyAfterMerge offers to put that right in
// one press. A local branch goes only when every commit on it reached the
// remote: see desktimers.CommitsOnlyHere for why git's own check cannot be
// used after a squash.

// mergedBranch is where the merged pull request's branch is.
type mergedBranch struct {
	// local is the local branch, "" when this repository has none.
	local        string
	remote       string
	remoteBranch string
	base         string
}

func (self *DesktimersHelper) mergedBranchOf(pr *models.GithubPullRequest) mergedBranch {
	merged := mergedBranch{remote: "origin", remoteBranch: pr.HeadRefName, base: pr.BaseRefName}
	for _, branch := range self.c.Model().Branches {
		if branch.IsTrackingRemote() && branch.UpstreamBranch == pr.HeadRefName {
			merged.local, merged.remote = branch.Name, branch.UpstreamRemote
			return merged
		}
	}
	return merged
}

// TidyAfterMerge asks what to clear away after pr was merged. Cancel
// leaves everything as it is.
func (self *DesktimersHelper) TidyAfterMerge(pr *models.GithubPullRequest) error {
	tr := self.c.Tr
	merged := self.mergedBranchOf(pr)
	words := map[string]string{"branch": merged.remoteBranch, "remote": merged.remote, "base": merged.base}
	items := []*types.MenuItem{}
	if merged.local != "" {
		words["branch"] = merged.local
		label := tr.PRTidyDelete
		branches := self.c.Model().Branches
		onIt := len(branches) > 0 && branches[0].Head && branches[0].Name == merged.local
		if onIt {
			label = tr.PRTidySwitchAndDelete
		}
		items = append(items, &types.MenuItem{
			LabelColumns: []string{utils.ResolvePlaceholderString(label, words)},
			OnPress:      func() error { return self.tidy(pr, merged, onIt, true) },
			Keys:         menuKey('1'),
		})
	}
	items = append(items, &types.MenuItem{
		LabelColumns: []string{utils.ResolvePlaceholderString(tr.PRTidyDeleteRemote, words)},
		OnPress:      func() error { return self.tidy(pr, merged, false, false) },
		Keys:         menuKey(rune('1' + len(items))),
	})
	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(tr.PRTidyTitle, map[string]string{"number": fmt.Sprintf("%d", pr.Number)}),
		Items: items,
	})
}

func (self *DesktimersHelper) tidy(pr *models.GithubPullRequest, merged mergedBranch, switchToBase bool, deleteLocal bool) error {
	tr := self.c.Tr
	return self.c.WithWaitingStatus(tr.PRTidyWorking, func(task gocui.Task) error {
		defer self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC})

		// Asked first: deleting the remote branch takes away what the
		// local one is compared with.
		onlyHere, compareErr := 0, error(nil)
		if deleteLocal {
			onlyHere, compareErr = desktimers.CommitsOnlyHere(".", merged.local, merged.remote+"/"+merged.remoteBranch)
		}

		if switchToBase {
			if err := self.c.Git().Branch.Checkout(merged.base, git_commands.CheckoutOptions{}); err != nil {
				return err
			}
			if err := self.c.Git().Sync.Pull(task, git_commands.PullOptions{FastForwardOnly: true}); err != nil {
				return err
			}
		}
		// GitHub may have deleted it already, on the repository's setting.
		if err := self.c.Git().Remote.DeleteRemoteBranch(task, merged.remote, []string{merged.remoteBranch}); err != nil &&
			!strings.Contains(err.Error(), "remote ref does not exist") {
			return err
		}

		if deleteLocal {
			kept := ""
			words := map[string]string{"branch": merged.local, "remote": merged.remote, "count": fmt.Sprintf("%d", onlyHere)}
			switch {
			case compareErr != nil:
				kept = utils.ResolvePlaceholderString(tr.PRTidyKeptNoRemote, words)
			case onlyHere > 0:
				kept = utils.ResolvePlaceholderString(tr.PRTidyKeptCommits, words)
			default:
				// Forced: after a squash git does not count it as merged.
				if err := self.c.Git().Branch.LocalDelete([]string{merged.local}, true); err != nil {
					return err
				}
			}
			if kept != "" {
				self.c.OnUIThread(func() error {
					self.c.Alert(utils.ResolvePlaceholderString(tr.PRTidyKeptTitle, words), kept)
					return nil
				})
				return nil
			}
		}
		self.c.Toast(utils.ResolvePlaceholderString(tr.PRTidyDone, map[string]string{"number": fmt.Sprintf("%d", pr.Number)}))
		return nil
	})
}

// TellDeskTickOfPullRequest puts a pull request deskgit just made, saved or
// merged on its task's card at once, in the background: GitHub's webhook
// says the same a little later, so a failure here is only logged.
func (self *DesktimersHelper) TellDeskTickOfPullRequest(notice desktimers.PullRequestNotice) {
	notice.Repo = desktimers.RepoSlug(".")
	if notice.Repo == "" {
		return
	}
	if notice.TaskCode == nil {
		code := desktimers.ExtractCode(notice.Head)
		if code == "" {
			code = desktimers.ExtractCode(notice.Title)
		}
		if code == "" {
			if picked := self.SelectedTask(); picked != nil {
				code = picked.Code
			}
		}
		if code != "" {
			notice.TaskCode = &code
		}
	}
	go func() {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			self.c.Log.Debugf("deskgit: pull request not told to DeskTick: %v", err)
			return
		}
		if _, err := client.SendPullRequest(notice); err != nil {
			self.c.Log.Debugf("deskgit: pull request not told to DeskTick: %v", err)
		}
	}()
}
