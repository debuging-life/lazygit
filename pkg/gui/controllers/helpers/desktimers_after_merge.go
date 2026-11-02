package helpers

import (
	"fmt"
	"slices"
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

// TidyAfterMerge asks what to clear away of branch, whose pull request pr
// was just merged. Cancel leaves everything as it is. A main branch is
// never offered for deletion, whatever it was merged from.
func (self *DesktimersHelper) TidyAfterMerge(pr *models.GithubPullRequest, branch *models.Branch) error {
	tr := self.c.Tr
	if !branch.IsTrackingRemote() || slices.Contains(self.c.UserConfig().Git.MainBranches, branch.Name) ||
		slices.Contains(self.c.UserConfig().Git.MainBranches, branch.UpstreamBranch) {
		return nil
	}
	words := map[string]string{"branch": branch.Name, "remote": branch.UpstreamRemote, "base": pr.BaseRefName}
	// Asked now, not when o was pressed: the merge refreshed the branches.
	onIt := false
	if branches := self.c.Model().Branches; len(branches) > 0 {
		onIt = branches[0].Head && branches[0].Name == branch.Name
	}
	label := tr.PRTidyDelete
	if onIt {
		label = tr.PRTidySwitchAndDelete
	}
	remoteWords := map[string]string{"branch": branch.UpstreamBranch, "remote": branch.UpstreamRemote}
	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(tr.PRTidyTitle, map[string]string{"number": fmt.Sprintf("%d", pr.Number)}),
		Items: []*types.MenuItem{
			{
				LabelColumns: []string{utils.ResolvePlaceholderString(label, words)},
				OnPress:      func() error { return self.tidy(pr, branch, onIt, true) },
				Keys:         menuKey('1'),
			},
			{
				LabelColumns: []string{utils.ResolvePlaceholderString(tr.PRTidyDeleteRemote, remoteWords)},
				OnPress:      func() error { return self.tidy(pr, branch, false, false) },
				Keys:         menuKey('2'),
			},
		},
	})
}

// tidy moves to the base first, so that changes in the way stop it before
// anything is deleted; then deletes the remote branch and the local one,
// and pulls the base last: a pull that fails (no upstream, diverged,
// offline) is reported with everything else already done.
func (self *DesktimersHelper) tidy(pr *models.GithubPullRequest, branch *models.Branch, switchToBase bool, deleteLocal bool) error {
	tr := self.c.Tr
	return self.c.WithWaitingStatus(tr.PRTidyWorking, func(task gocui.Task) error {
		defer self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC})

		// Asked first: deleting the remote branch takes away what the
		// local one is compared with.
		onlyHere, compareErr := 0, error(nil)
		if deleteLocal {
			onlyHere, compareErr = desktimers.CommitsOnlyHere(".", branch.Name, branch.UpstreamRemote+"/"+branch.UpstreamBranch)
		}

		if switchToBase {
			if err := self.c.Git().Branch.Checkout(pr.BaseRefName, git_commands.CheckoutOptions{}); err != nil {
				return err
			}
		}
		// GitHub may have deleted it already, on the repository's setting.
		// (Pushes run with LANG=C, so git says this in English.)
		if err := self.c.Git().Remote.DeleteRemoteBranch(task, branch.UpstreamRemote, []string{branch.UpstreamBranch}); err != nil &&
			!strings.Contains(err.Error(), "remote ref does not exist") {
			return err
		}

		kept := ""
		words := map[string]string{"branch": branch.Name, "remote": branch.UpstreamRemote, "count": fmt.Sprintf("%d", onlyHere)}
		if deleteLocal {
			switch {
			case compareErr != nil:
				kept = utils.ResolvePlaceholderString(tr.PRTidyKeptNoRemote, words)
			case onlyHere > 0:
				kept = utils.ResolvePlaceholderString(tr.PRTidyKeptCommits, words)
			default:
				// Forced: after a squash git does not count it as merged.
				if err := self.c.Git().Branch.LocalDelete([]string{branch.Name}, true); err != nil {
					return err
				}
			}
		}

		// Told whatever the pull does: a branch kept for its commits must
		// not go unmentioned behind a pull's error.
		if kept != "" {
			self.c.OnUIThread(func() error {
				self.c.Alert(utils.ResolvePlaceholderString(tr.PRTidyKeptTitle, words), kept)
				return nil
			})
		}
		if switchToBase {
			if err := self.c.Git().Sync.Pull(task, git_commands.PullOptions{FastForwardOnly: true}); err != nil {
				return err
			}
		}
		if kept != "" {
			return nil
		}
		self.c.Toast(utils.ResolvePlaceholderString(tr.PRTidyDone, map[string]string{"number": fmt.Sprintf("%d", pr.Number)}))
		return nil
	})
}

// TellDeskTickOfPullRequest puts a pull request deskgit just made, saved or
// merged on its task's card at once, in the background: GitHub's webhook
// says the same a little later, so a failure here is only logged. The API
// reads codes from the title and the branch; TaskCode is only for the task
// picked when a pull request naming none was made.
func (self *DesktimersHelper) TellDeskTickOfPullRequest(notice desktimers.PullRequestNotice) {
	notice.Repo = desktimers.RepoSlug(".")
	if notice.Repo == "" {
		return
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
