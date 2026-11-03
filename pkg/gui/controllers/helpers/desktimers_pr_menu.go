package helpers

import (
	"errors"
	"fmt"

	"github.com/jesseduffield/lazygit/pkg/commands/hosting_service"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

// ── what can be done with a pull request ────────────────────────────────────
// o on a branch: the pull request form for a new one, or for one already
// open, the menu that opens, copies, edits and merges it, and then tidies
// up after the merge.

// PullRequestPlace is where a pull request is: which GitHub, asked as
// whom, and the local branch it is from. The branches panel knows it.
type PullRequestPlace struct {
	Service hosting_service.ServiceInfo
	Token   string
	// Branch is the local branch the pull request is from.
	Branch *models.Branch
	// EditFile opens a file in the person's editor and returns when it is
	// closed.
	EditFile        func(path string) error
	BaseSuggestions func(string) []*types.Suggestion
	// Reopen starts again from the branches panel, after another gh
	// account was chosen: another account is another token.
	Reopen func() error
}

func (p PullRequestPlace) repo() desktimers.GitHubRepo {
	return desktimers.GitHubRepo{
		Host: p.Service.WebDomain, Owner: p.Service.Owner, Name: p.Service.Repository, Token: p.Token,
	}
}

// head is the branch on GitHub the local branch is pushed to.
func (p PullRequestPlace) head() string {
	if p.Branch.IsTrackingRemote() {
		return p.Branch.UpstreamBranch
	}
	return p.Branch.Name
}

// CreatePullRequest opens the form for a new pull request from the
// branch, into base ("" for the repository's default); once made, its
// menu.
func (self *DesktimersHelper) CreatePullRequest(place PullRequestPlace, base string) error {
	if !place.Branch.IsTrackingRemote() {
		return errors.New(self.c.Tr.PullRequestNoUpstream)
	}
	return self.OpenPullRequestForm(PullRequestFormOpts{
		Repo: place.repo(), Branch: place.Branch.Name, Head: place.head(), Base: base,
		EditFile: place.EditFile, BaseSuggestions: place.BaseSuggestions, Reopen: place.Reopen,
		OnDone: func(result PullRequestResult) error {
			pr := pullRequestOf(result, nil)
			title := utils.ResolvePlaceholderString(self.c.Tr.PRCreatedMenuTitle, map[string]string{"number": fmt.Sprintf("%d", pr.Number)})
			return self.showPullRequestMenu(place, pr, title, result.Note)
		},
	})
}

// ShowPullRequestMenu is the menu for the branch's pull request, open or a
// draft, with where it stands above the choices.
func (self *DesktimersHelper) ShowPullRequestMenu(place PullRequestPlace, pr *models.GithubPullRequest) error {
	return self.showPullRequestMenu(place, pr, self.existingTitle(pr), self.PullRequestStanding(pr))
}

func (self *DesktimersHelper) existingTitle(pr *models.GithubPullRequest) string {
	return utils.ResolvePlaceholderString(self.c.Tr.PRExistingMenuTitle, map[string]string{
		"number": fmt.Sprintf("%d", pr.Number), "head": pr.HeadRefName, "base": pr.BaseRefName,
	})
}

// pullRequestOf is the pull request as the form left it; what GitHub says
// of its reviews is kept from before, the form not having changed it, and
// of a conflict too unless the base changed.
func pullRequestOf(result PullRequestResult, before *models.GithubPullRequest) *models.GithubPullRequest {
	pr := &models.GithubPullRequest{}
	if before != nil {
		*pr = *before
	}
	if before != nil && before.BaseRefName != result.Draft.Base {
		// Whether it conflicts was with the old base.
		pr.Conflicting = false
	}
	pr.Number, pr.Url, pr.Title = result.Number, result.URL, result.Draft.Title
	pr.HeadRefName, pr.BaseRefName = result.Draft.Head, result.Draft.Base
	pr.State = "OPEN"
	if result.Draft.Draft {
		pr.State = "DRAFT"
	}
	return pr
}

// note is shown above the choices: where the pull request stands, or what
// GitHub refused of the rest of a form just sent.
func (self *DesktimersHelper) showPullRequestMenu(place PullRequestPlace, pr *models.GithubPullRequest, title string, note string) error {
	tr := self.c.Tr
	merge := &types.MenuItem{
		LabelColumns: []string{tr.PRCreatedMergeInApp},
		OnPress:      func() error { return self.mergePullRequest(place, pr) },
	}
	if pr.State == "DRAFT" {
		merge.DisabledReason = &types.DisabledReason{Text: tr.PRMergeDraft}
	}
	return self.c.Menu(types.CreateMenuOptions{
		Title:  title,
		Prompt: note,
		Items: []*types.MenuItem{
			{
				LabelColumns: []string{tr.PRCreatedOpenInBrowser},
				OnPress:      func() error { return self.c.OS().OpenLink(pr.Url) },
			},
			{
				LabelColumns: []string{tr.PRCreatedCopyLink},
				OnPress: func() error {
					if err := self.c.OS().CopyToClipboard(pr.Url); err != nil {
						return err
					}
					self.c.Toast(tr.PullRequestURLCopiedToClipboard)
					return nil
				},
			},
			{
				LabelColumns: []string{tr.PRMenuEdit},
				OnPress:      func() error { return self.editPullRequest(place, pr) },
			},
			merge,
		},
	})
}

// editPullRequest is the form on the open pull request as GitHub has it.
// After saving, its menu comes back, showing it as saved.
func (self *DesktimersHelper) editPullRequest(place PullRequestPlace, pr *models.GithubPullRequest) error {
	return self.OpenPullRequestForm(PullRequestFormOpts{
		Repo: place.repo(), Number: pr.Number, Branch: place.Branch.Name, Head: pr.HeadRefName,
		EditFile: place.EditFile, BaseSuggestions: place.BaseSuggestions, Reopen: place.Reopen,
		OnDone: func(result PullRequestResult) error {
			saved := pullRequestOf(result, pr)
			note := result.Note
			if note == "" {
				self.c.Toast(utils.ResolvePlaceholderString(self.c.Tr.PRFormSaved, map[string]string{"number": fmt.Sprintf("%d", saved.Number)}))
				note = self.PullRequestStanding(saved)
			}
			return self.showPullRequestMenu(place, saved, self.existingTitle(saved), note)
		},
	})
}

// mergePullRequest asks how, merges, and offers to tidy up. The task is
// not moved from here: DeskTick moves it to Done itself when GitHub says
// the pull request merged, if its title or branch names the task.
func (self *DesktimersHelper) mergePullRequest(place PullRequestPlace, pr *models.GithubPullRequest) error {
	tr := self.c.Tr
	strategy := func(label string, strategy string) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: []string{label},
			OnPress: func() error {
				self.c.LogAction(tr.Actions.MergePullRequest)
				return self.c.WithWaitingStatus(tr.PRMergingPR, func(gocui.Task) error {
					svc := place.Service
					if err := self.c.Git().GitHub.MergePullRequest(svc.Owner, svc.Repository, pr.Number, strategy, svc.WebDomain, place.Token); err != nil {
						return err
					}
					self.c.Refresh(types.RefreshOptions{Scope: []types.RefreshableView{types.BRANCHES}})
					self.c.Toast(tr.PRMerged)
					self.TellDeskTickOfPullRequest(desktimers.PullRequestNotice{
						Number: pr.Number, Title: pr.Title, URL: pr.Url, State: desktimers.PullMerged,
						Head: pr.HeadRefName, Base: pr.BaseRefName,
					})
					self.c.OnUIThread(func() error { return self.TidyAfterMerge(pr, place.Branch) })
					return nil
				})
			},
		}
	}
	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(tr.PRMergeConfirmTitle, map[string]string{"number": fmt.Sprintf("%d", pr.Number)}),
		Items: []*types.MenuItem{
			strategy(tr.PRMergeStrategyMerge, "merge"),
			strategy(tr.PRMergeStrategySquash, "squash"),
			strategy(tr.PRMergeStrategyRebase, "rebase"),
		},
	})
}
