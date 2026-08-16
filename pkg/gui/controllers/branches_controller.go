package controllers

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gookit/color"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/hosting_service"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/context"
	"github.com/jesseduffield/lazygit/pkg/gui/controllers/helpers"
	"github.com/jesseduffield/lazygit/pkg/gui/presentation"
	"github.com/jesseduffield/lazygit/pkg/gui/presentation/icons"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"github.com/samber/lo"
)

type BranchesController struct {
	baseController
	*ListControllerTrait[*models.Branch]
	c *ControllerCommon
}

var _ types.IController = &BranchesController{}

func NewBranchesController(
	c *ControllerCommon,
) *BranchesController {
	return &BranchesController{
		baseController: baseController{},
		c:              c,
		ListControllerTrait: NewListControllerTrait(
			c,
			c.Contexts().Branches,
			c.Contexts().Branches.GetSelected,
			c.Contexts().Branches.GetSelectedItems,
		),
	}
}

func (self *BranchesController) GetKeybindings(opts types.KeybindingsOpts) []*types.Binding {
	return []*types.Binding{
		{
			Keys:    opts.GetKeys(opts.Config.Universal.Select),
			Handler: self.withItem(self.press),
			GetDisabledReason: self.require(
				self.singleItemSelected(),
				self.notPulling,
			),
			Description:     self.c.Tr.Checkout,
			Tooltip:         self.c.Tr.CheckoutTooltip,
			DisplayOnScreen: true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Universal.New),
			Handler:           self.withItem(self.newBranch),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.NewBranch,
			DisplayOnScreen:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.MoveCommitsToNewBranch),
			Handler:           self.c.Helpers().Refs.MoveCommitsToNewBranch,
			GetDisabledReason: self.c.Helpers().Refs.CanMoveCommitsToNewBranch,
			Description:       self.c.Tr.MoveCommitsToNewBranch,
			Tooltip:           self.c.Tr.MoveCommitsToNewBranchTooltip,
		},
		{
			Keys:        opts.GetKeys(opts.Config.Universal.NewWorktree),
			Handler:     self.withItem(self.c.Helpers().Worktree.NewWorktreeMenuForBranch),
			Description: self.c.Tr.NewWorktree,
			OpensMenu:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.CreatePullRequest),
			Handler:           self.withItem(self.handleCreatePullRequest),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.CreatePullRequest,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.ViewPullRequestOptions),
			Handler:           self.withItem(self.handleCreatePullRequestMenu),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.CreatePullRequestOptions,
			OpensMenu:         true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.OpenPullRequestInBrowser),
			Handler:           self.withItem(self.openPRInBrowser),
			GetDisabledReason: self.require(self.singleItemSelected(self.branchHasPR)),
			Description:       self.c.Tr.OpenPullRequestInBrowser,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.CopyPullRequestURL),
			Handler:           self.copyPullRequestURL,
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.CopyPullRequestURL,
		},
		{
			Keys:        opts.GetKeys(opts.Config.Branches.CheckoutBranchByName),
			Handler:     self.checkoutByName,
			Description: self.c.Tr.CheckoutByName,
			Tooltip:     self.c.Tr.CheckoutByNameTooltip,
		},
		{
			Keys:        opts.GetKeys(opts.Config.Branches.CheckoutPreviousBranch),
			Handler:     self.checkoutPreviousBranch,
			Description: self.c.Tr.CheckoutPreviousBranch,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.ForceCheckoutBranch),
			Handler:           self.forceCheckout,
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.ForceCheckout,
			Tooltip:           self.c.Tr.ForceCheckoutTooltip,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Universal.Remove),
			Handler:           self.withItems(self.delete),
			GetDisabledReason: self.require(self.itemRangeSelected(self.branchesAreReal)),
			Description:       self.c.Tr.Delete,
			Tooltip:           self.c.Tr.BranchDeleteTooltip,
			OpensMenu:         true,
			DisplayOnScreen:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.RebaseBranch),
			Handler:           opts.Guards.OutsideFilterMode(self.withItem(self.rebase)),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.RebaseBranch,
			Tooltip:           self.c.Tr.RebaseBranchTooltip,
			OpensMenu:         true,
			DisplayOnScreen:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.MergeIntoCurrentBranch),
			Handler:           opts.Guards.OutsideFilterMode(self.merge),
			GetDisabledReason: self.require(self.singleItemSelected(self.notMergingIntoYourself)),
			Description:       self.c.Tr.Merge,
			Tooltip:           self.c.Tr.MergeBranchTooltip,
			DisplayOnScreen:   true,
			OpensMenu:         true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.FastForward),
			Handler:           self.withItem(self.fastForward),
			GetDisabledReason: self.require(self.singleItemSelected(self.branchIsReal)),
			Description:       self.c.Tr.FastForward,
			Tooltip:           self.c.Tr.FastForwardTooltip,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.CreateTag),
			Handler:           self.withItem(self.createTag),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.NewTag,
		},
		{
			Keys:        opts.GetKeys(opts.Config.Branches.SortOrder),
			Handler:     self.createSortMenu,
			Description: self.c.Tr.SortOrder,
			OpensMenu:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Commits.ViewResetOptions),
			Handler:           self.withItem(self.createResetMenu),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.ViewResetOptions,
			OpensMenu:         true,
			DisplayOnScreen:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.RenameBranch),
			Handler:           self.withItem(self.rename),
			GetDisabledReason: self.require(self.singleItemSelected(self.branchIsReal)),
			Description:       self.c.Tr.RenameBranch,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Branches.SetUpstream),
			Handler:           self.withItem(self.viewUpstreamOptions),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.ViewBranchUpstreamOptions,
			Tooltip:           self.c.Tr.ViewBranchUpstreamOptionsTooltip,
			ShortDescription:  self.c.Tr.Upstream,
			OpensMenu:         true,
			DisplayOnScreen:   true,
		},
		{
			Keys: opts.GetKeys(opts.Config.Universal.OpenDiffTool),
			Handler: self.withItem(func(selectedBranch *models.Branch) error {
				return self.c.Helpers().Diff.OpenDiffToolForRef(selectedBranch)
			}),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.OpenDiffTool,
		},
	}
}

func (self *BranchesController) GetOnRenderToMain() func() {
	return func() {
		self.c.Helpers().Diff.WithDiffModeCheck(func() {
			var task types.UpdateTask
			branch := self.context().GetSelected()
			if branch == nil {
				task = types.NewRenderStringTask(self.c.Tr.NoBranchesThisRepo)
			} else {
				cmdObj := self.c.Git().Branch.GetGraphCmdObj(branch.FullRefName())

				ptyTask := types.NewRunPtyTask(cmdObj.GetCmd())
				task = ptyTask

				pr, ok := self.c.Model().PullRequestsMap[branch.Name]
				if ok && presentation.ShouldShowPrForBranch(pr, branch.Name, self.c.UserConfig()) {
					icon := lo.Ternary(icons.IsIconEnabled(), icons.IconForRemoteUrl(pr.Url)+"  ", "")
					ptyTask.Prefix = style.PrintHyperlink(fmt.Sprintf("%s%s  %s  %s\n",
						icon,
						coloredStateText(pr.State),
						pr.Title,
						style.FgCyan.Sprintf("#%d", pr.Number)),
						pr.Url)
					ptyTask.Prefix += strings.Repeat("─", self.c.Contexts().Normal.GetView().InnerWidth()) + "\n"
				}
			}

			self.c.RenderToMainViews(types.RefreshMainOpts{
				Pair: self.c.MainViewPairs().Normal,
				Main: &types.ViewUpdateOpts{
					Title: self.c.Tr.LogTitle,
					Task:  task,
				},
			})
		})
	}
}

func stateText(state string) string {
	var icon, label string
	switch state {
	case "OPEN":
		icon, label = " ", "Open"
	case "CLOSED":
		icon, label = " ", "Closed"
	case "MERGED":
		icon, label = " ", "Merged"
	case "DRAFT":
		icon, label = " ", "Draft"
	default:
		return ""
	}
	if icons.IsIconEnabled() {
		return icon + label
	}
	return label
}

func coloredStateText(state string) string {
	if icons.IsIconEnabled() {
		return fmt.Sprintf("%s%s%s",
			presentation.WithPrColor(state, "", false),
			presentation.WithPrColor(state, color.RGB(0xFF, 0xFF, 0xFF, false).Sprint(stateText(state)), true),
			presentation.WithPrColor(state, "", false))
	}

	return presentation.WithPrColor(state, stateText(state), false)
}

func (self *BranchesController) viewUpstreamOptions(selectedBranch *models.Branch) error {
	upstream := lo.Ternary(selectedBranch.RemoteBranchStoredLocally(),
		selectedBranch.ShortUpstreamRefName(),
		self.c.Tr.UpstreamGenericName)

	viewDivergenceItem := &types.MenuItem{
		LabelColumns: []string{self.c.Tr.ViewDivergenceFromUpstream},
		OnPress: func() error {
			branch := self.context().GetSelected()
			if branch == nil {
				return nil
			}

			return self.c.Helpers().SubCommits.ViewSubCommits(helpers.ViewSubCommitsOpts{
				Ref:                     branch,
				TitleRef:                fmt.Sprintf("%s <-> %s", branch.RefName(), upstream),
				RefToShowDivergenceFrom: branch.FullUpstreamRefName(),
				Context:                 self.context(),
				ShowBranchHeads:         false,
			})
		},
	}

	var disabledReason *types.DisabledReason
	baseBranch, err := self.c.Git().Loaders.BranchLoader.GetBaseBranch(selectedBranch, self.c.Model().MainBranches)
	if err != nil {
		return err
	}
	if baseBranch == "" {
		baseBranch = self.c.Tr.CouldNotDetermineBaseBranch
		disabledReason = &types.DisabledReason{Text: self.c.Tr.CouldNotDetermineBaseBranch}
	}
	shortBaseBranchName := helpers.ShortBranchName(baseBranch)
	label := utils.ResolvePlaceholderString(
		self.c.Tr.ViewDivergenceFromBaseBranch,
		map[string]string{"baseBranch": shortBaseBranchName},
	)
	viewDivergenceFromBaseBranchItem := &types.MenuItem{
		LabelColumns: []string{label},
		Keys:         menuKey('b'),
		OnPress: func() error {
			branch := self.context().GetSelected()
			if branch == nil {
				return nil
			}

			return self.c.Helpers().SubCommits.ViewSubCommits(helpers.ViewSubCommitsOpts{
				Ref:                     branch,
				TitleRef:                fmt.Sprintf("%s <-> %s", branch.RefName(), shortBaseBranchName),
				RefToShowDivergenceFrom: baseBranch,
				Context:                 self.context(),
				ShowBranchHeads:         false,
			})
		},
		DisabledReason: disabledReason,
	}

	unsetUpstreamItem := &types.MenuItem{
		LabelColumns: []string{self.c.Tr.UnsetUpstream},
		OnPress: func() error {
			if err := self.c.Git().Branch.UnsetUpstream(selectedBranch.Name); err != nil {
				return err
			}
			self.c.Refresh(types.RefreshOptions{
				Mode: types.SYNC,
				Scope: []types.RefreshableView{
					types.BRANCHES,
					types.COMMITS,
				},
			})
			return nil
		},
		Keys: menuKey('u'),
	}

	setUpstreamItem := &types.MenuItem{
		LabelColumns: []string{self.c.Tr.SetUpstream},
		OnPress: func() error {
			return self.c.Helpers().Upstream.PromptForUpstreamWithoutInitialContent(selectedBranch, func(upstream string) error {
				upstreamRemote, upstreamBranch, err := self.c.Helpers().Upstream.ParseUpstream(upstream)
				if err != nil {
					return err
				}

				if err := self.c.Git().Branch.SetUpstream(upstreamRemote, upstreamBranch, selectedBranch.Name); err != nil {
					return err
				}
				self.c.Refresh(types.RefreshOptions{
					Mode: types.SYNC,
					Scope: []types.RefreshableView{
						types.BRANCHES,
						types.COMMITS,
					},
				})
				return nil
			})
		},
		Keys: menuKey('s'),
	}

	upstreamResetOptions := utils.ResolvePlaceholderString(
		self.c.Tr.ViewUpstreamResetOptions,
		map[string]string{"upstream": upstream},
	)
	upstreamResetTooltip := utils.ResolvePlaceholderString(
		self.c.Tr.ViewUpstreamResetOptionsTooltip,
		map[string]string{"upstream": upstream},
	)

	upstreamRebaseOptions := utils.ResolvePlaceholderString(
		self.c.Tr.ViewUpstreamRebaseOptions,
		map[string]string{"upstream": upstream},
	)
	upstreamRebaseTooltip := utils.ResolvePlaceholderString(
		self.c.Tr.ViewUpstreamRebaseOptionsTooltip,
		map[string]string{"upstream": upstream},
	)

	upstreamResetItem := &types.MenuItem{
		LabelColumns: []string{upstreamResetOptions},
		OpensMenu:    true,
		OnPress: func() error {
			// We only can invoke this when the remote branch is stored locally, so using the selectedBranch here is fine.
			err := self.c.Helpers().Refs.CreateGitResetMenu(upstream, selectedBranch.FullUpstreamRefName())
			if err != nil {
				return err
			}
			return nil
		},
		Tooltip: upstreamResetTooltip,
		Keys:    menuKey('g'),
	}

	upstreamRebaseItem := &types.MenuItem{
		LabelColumns: []string{upstreamRebaseOptions},
		OpensMenu:    true,
		OnPress: func() error {
			if err := self.c.Helpers().MergeAndRebase.RebaseOntoRef(upstream); err != nil {
				return err
			}
			return nil
		},
		Tooltip: upstreamRebaseTooltip,
		Keys:    menuKey('r'),
	}

	if !selectedBranch.IsTrackingRemote() {
		unsetUpstreamItem.DisabledReason = &types.DisabledReason{Text: self.c.Tr.UpstreamNotSetError}
	}

	if !selectedBranch.RemoteBranchStoredLocally() {
		viewDivergenceItem.DisabledReason = &types.DisabledReason{Text: self.c.Tr.UpstreamNotSetError}
		upstreamResetItem.DisabledReason = &types.DisabledReason{Text: self.c.Tr.UpstreamNotSetError}
		upstreamRebaseItem.DisabledReason = &types.DisabledReason{Text: self.c.Tr.UpstreamNotSetError}
	}

	options := []*types.MenuItem{
		viewDivergenceItem,
		viewDivergenceFromBaseBranchItem,
		unsetUpstreamItem,
		setUpstreamItem,
		upstreamResetItem,
		upstreamRebaseItem,
	}

	return self.c.Menu(types.CreateMenuOptions{
		Title: self.c.Tr.BranchUpstreamOptionsTitle,
		Items: options,
	})
}

func (self *BranchesController) Context() types.Context {
	return self.context()
}

func (self *BranchesController) context() *context.BranchesContext {
	return self.c.Contexts().Branches
}

func (self *BranchesController) press(selectedBranch *models.Branch) error {
	if selectedBranch == self.c.Helpers().Refs.GetCheckedOutRef() {
		return errors.New(self.c.Tr.AlreadyCheckedOutBranch)
	}

	worktreeForRef, ok := self.worktreeForBranch(selectedBranch)
	if ok && !worktreeForRef.IsCurrent {
		return self.promptToCheckoutWorktree(worktreeForRef)
	}

	self.c.LogAction(self.c.Tr.Actions.CheckoutBranch)
	return self.c.Helpers().Refs.CheckoutRef(selectedBranch.Name, types.CheckoutRefOptions{})
}

func (self *BranchesController) notPulling() *types.DisabledReason {
	currentBranch := self.c.Helpers().Refs.GetCheckedOutRef()
	if currentBranch != nil {
		op := self.c.State().GetItemOperation(currentBranch)
		if op == types.ItemOperationFastForwarding || op == types.ItemOperationPulling {
			return &types.DisabledReason{Text: self.c.Tr.CantCheckoutBranchWhilePulling}
		}
	}

	return nil
}

func (self *BranchesController) worktreeForBranch(branch *models.Branch) (*models.Worktree, bool) {
	return git_commands.WorktreeForBranch(branch, self.c.Model().Worktrees)
}

func (self *BranchesController) promptToCheckoutWorktree(worktree *models.Worktree) error {
	prompt := utils.ResolvePlaceholderString(self.c.Tr.AlreadyCheckedOutByWorktree, map[string]string{
		"worktreeName": worktree.Name,
	})

	return self.c.ConfirmIf(!self.c.UserConfig().Gui.SkipSwitchWorktreeOnCheckoutWarning, types.ConfirmOpts{
		Title:  self.c.Tr.SwitchToWorktree,
		Prompt: prompt,
		HandleConfirm: func() error {
			return self.c.Helpers().Worktree.Switch(worktree, context.LOCAL_BRANCHES_CONTEXT_KEY)
		},
	})
}

func (self *BranchesController) handleCreatePullRequest(selectedBranch *models.Branch) error {
	if !selectedBranch.IsTrackingRemote() {
		return errors.New(self.c.Tr.PullRequestNoUpstream)
	}
	return self.createPullRequest(selectedBranch.UpstreamBranch, "")
}

// getGitHubAuth returns GitHub service info and token, or (zero, zero, false) if
// GitHub is not configured or no token is available.
func (self *BranchesController) getGitHubAuth() (hosting_service.ServiceInfo, string, bool) {
	svcInfo, err := self.c.Helpers().Host.GetServiceInfo()
	if err != nil || svcInfo.Provider != "github" {
		return hosting_service.ServiceInfo{}, "", false
	}
	token := self.c.Git().GitHub.GetAuthToken(svcInfo.WebDomain)
	if token == "" {
		return hosting_service.ServiceInfo{}, "", false
	}
	return svcInfo, token, true
}

// handleCreatePRViaAPI shows the base-branch picker, pre-fills title/body from
// the last commit, then creates the PR via the GitHub API.
func (self *BranchesController) handleCreatePRViaAPI(branch *models.Branch) error {
	svcInfo, token, ok := self.getGitHubAuth()
	if !ok {
		return self.handleCreatePullRequest(branch)
	}
	self.c.Prompt(types.PromptOpts{
		Title:               self.c.Tr.PRBaseBranch,
		FindSuggestionsFunc: self.c.Helpers().Suggestions.GetRemoteBranchesForRemoteSuggestionsFunc("origin"),
		HandleConfirm: func(baseBranch string) error {
			return self.createPRViaAPIWithBase(branch, baseBranch, svcInfo, token)
		},
	})
	return nil
}

// handleCreatePRViaAPIToDefault creates a PR targeting the repo's default branch.
func (self *BranchesController) handleCreatePRViaAPIToDefault(branch *models.Branch) error {
	svcInfo, token, ok := self.getGitHubAuth()
	if !ok {
		return self.handleCreatePullRequest(branch)
	}
	return self.createPRViaAPIWithBase(branch, "", svcInfo, token)
}

// createPRViaAPIWithBase prompts for title/body (pre-filled from last commit),
// then calls the GitHub REST API to create the PR with a progress spinner.
func (self *BranchesController) createPRViaAPIWithBase(branch *models.Branch, baseBranch string, svcInfo hosting_service.ServiceInfo, token string) error {
	headBranch := branch.Name
	if branch.IsTrackingRemote() {
		headBranch = branch.UpstreamBranch
	}

	// Guard: strip any "origin/" prefix for comparison, then reject head == base.
	normalised := strings.TrimPrefix(baseBranch, "origin/")
	if normalised != "" && normalised == strings.TrimPrefix(headBranch, "origin/") {
		return errors.New(self.c.Tr.PRSameBranch)
	}

	// Pre-fill from the last commit message.
	defaultTitle := ""
	defaultBody := ""
	if msg, err := self.c.Git().Commit.GetCommitMessage(branch.CommitHash); err == nil && msg != "" {
		parts := strings.SplitN(msg, "\n", 2)
		defaultTitle = parts[0]
		if len(parts) > 1 {
			defaultBody = strings.TrimLeft(parts[1], "\n")
		}
	}

	self.c.Prompt(types.PromptOpts{
		Title:          self.c.Tr.PRTitle,
		InitialContent: defaultTitle,
		HandleConfirm: func(title string) error {
			self.c.Prompt(types.PromptOpts{
				Title:          self.c.Tr.PRBody,
				InitialContent: defaultBody,
				HandleConfirm: func(body string) error {
					self.c.LogAction(self.c.Tr.Actions.CreatePullRequestViaAPI)
					return self.c.WithWaitingStatus(self.c.Tr.PRCreatingPR, func(_ gocui.Task) error {
						pr, err := self.c.Git().GitHub.CreatePullRequest(
							svcInfo.Owner, svcInfo.Repository,
							title, body, headBranch, baseBranch,
							svcInfo.WebDomain, token,
						)
						if err != nil {
							return err
						}
						self.c.Refresh(types.RefreshOptions{Scope: []types.RefreshableView{types.BRANCHES}})
						capturedPR := pr
						capturedSvc := svcInfo
						capturedTok := token
						menuTitle := utils.ResolvePlaceholderString(
							self.c.Tr.PRCreatedMenuTitle,
							map[string]string{"number": fmt.Sprintf("%d", pr.Number)},
						)
						return self.c.Menu(types.CreateMenuOptions{
							Title: menuTitle,
							Items: []*types.MenuItem{
								{
									LabelColumns: []string{self.c.Tr.PRCreatedOpenInBrowser},
									OnPress: func() error {
										return self.c.OS().OpenLink(capturedPR.Url)
									},
								},
								{
									LabelColumns: []string{self.c.Tr.PRCreatedCopyLink},
									OnPress: func() error {
										if err := self.c.OS().CopyToClipboard(capturedPR.Url); err != nil {
											return err
										}
										self.c.Toast(self.c.Tr.PullRequestURLCopiedToClipboard)
										return nil
									},
								},
								{
									LabelColumns: []string{self.c.Tr.PRCreatedMergeInApp},
									OnPress: func() error {
										return self.handleMergePRViaAPI(capturedPR, capturedSvc, capturedTok)
									},
								},
							},
						})
					})
				},
			})
			return nil
		},
	})
	return nil
}

func (self *BranchesController) handleMergePRViaAPI(pr *models.GithubPullRequest, svcInfo hosting_service.ServiceInfo, token string) error {
	// Resolve task code before doing anything else so we can ask "which column?"
	// BEFORE the merge — that way the suppress flag is set before the webhook fires.
	taskCode := desktimers.ExtractCode(pr.HeadRefName)
	if taskCode == "" {
		taskCode = desktimers.ExtractCode(pr.Title)
	}
	if taskCode == "" {
		if sel := self.c.Helpers().Desktimers.SelectedTask(); sel != nil {
			taskCode = sel.Code
		}
	}

	if taskCode == "" {
		// No linked task — skip the column picker, go straight to strategy.
		return self.showMergeStrategyPicker(pr, svcInfo, token, nil, "")
	}

	capturedCode := taskCode
	// Fetch project columns with a spinner, then show the column picker.
	return self.c.WithWaitingStatus(self.c.Tr.PRLoadingColumns, func(_ gocui.Task) error {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			return self.showMergeStrategyPicker(pr, svcInfo, token, nil, "")
		}
		columns, err := client.GetTaskColumns(capturedCode)
		if err != nil || len(columns) == 0 {
			return self.showMergeStrategyPicker(pr, svcInfo, token, nil, "")
		}
		return self.showColumnPickerMenu(pr, svcInfo, token, client, capturedCode, columns)
	})
}

// showColumnPickerMenu lets the user pick which column to move the task to
// after the merge. The choice is recorded BEFORE the merge so the suppress
// flag can prevent the webhook from also moving the task.
func (self *BranchesController) showColumnPickerMenu(
	pr *models.GithubPullRequest,
	svcInfo hosting_service.ServiceInfo,
	token string,
	client *desktimers.Client,
	taskCode string,
	columns []desktimers.Column,
) error {
	items := make([]*types.MenuItem, 0, len(columns)+1)
	for _, col := range columns {
		col := col // capture loop variable
		items = append(items, &types.MenuItem{
			LabelColumns: []string{col.Name},
			OnPress: func() error {
				// Suppress webhook auto-done, then merge with a post-merge column move.
				_ = client.SuppressAutoDone(taskCode)
				return self.showMergeStrategyPicker(pr, svcInfo, token, client, taskCode+":"+col.Slug)
			},
		})
	}
	items = append(items, &types.MenuItem{
		LabelColumns: []string{self.c.Tr.PRMoveToColumnKeep},
		OnPress: func() error {
			_ = client.SuppressAutoDone(taskCode)
			return self.showMergeStrategyPicker(pr, svcInfo, token, nil, "")
		},
	})
	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(
			self.c.Tr.PRMoveToColumnPrompt,
			map[string]string{"code": taskCode},
		),
		Items: items,
	})
}

// showMergeStrategyPicker shows merge/squash/rebase options.
// postMergeTarget is "CODE:columnSlug" when we should call MoveTaskToColumn
// after a successful merge; empty string means no column move.
func (self *BranchesController) showMergeStrategyPicker(
	pr *models.GithubPullRequest,
	svcInfo hosting_service.ServiceInfo,
	token string,
	client *desktimers.Client,
	postMergeTarget string,
) error {
	strategies := []struct {
		label    string
		strategy string
	}{
		{self.c.Tr.PRMergeStrategyMerge, "merge"},
		{self.c.Tr.PRMergeStrategySquash, "squash"},
		{self.c.Tr.PRMergeStrategyRebase, "rebase"},
	}
	menuItems := lo.Map(strategies, func(s struct {
		label    string
		strategy string
	}, _ int,
	) *types.MenuItem {
		strat := s.strategy
		return &types.MenuItem{
			LabelColumns: []string{s.label},
			OnPress: func() error {
				self.c.LogAction(self.c.Tr.Actions.MergePullRequest)
				return self.c.WithWaitingStatus(self.c.Tr.PRMergingPR, func(_ gocui.Task) error {
					if err := self.c.Git().GitHub.MergePullRequest(
						svcInfo.Owner, svcInfo.Repository, pr.Number, strat, svcInfo.WebDomain, token,
					); err != nil {
						return err
					}
					self.c.Refresh(types.RefreshOptions{Scope: []types.RefreshableView{types.BRANCHES}})
					self.c.Toast(self.c.Tr.PRMerged)
					// If the user chose a specific column, move the task there now.
					if postMergeTarget != "" && client != nil {
						parts := strings.SplitN(postMergeTarget, ":", 2)
						if len(parts) == 2 {
							code, slug := parts[0], parts[1]
							go func() { _ = client.MoveTaskToColumn(code, slug) }()
						}
					}
					return nil
				})
			},
		}
	})
	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(
			self.c.Tr.PRMergeConfirmTitle,
			map[string]string{"number": fmt.Sprintf("%d", pr.Number)},
		),
		Items: menuItems,
	})
}

func (self *BranchesController) handleCreatePullRequestMenu(selectedBranch *models.Branch) error {
	checkedOutBranch := self.c.Helpers().Refs.GetCheckedOutRef()

	return self.createPullRequestMenu(selectedBranch, checkedOutBranch)
}

func (self *BranchesController) getPullRequestURL() (string, error) {
	branch := self.context().GetSelected()
	if pr, ok := self.c.Model().PullRequestsMap[branch.Name]; ok {
		return pr.Url, nil
	}

	branchExistsOnRemote := self.c.Git().Remote.CheckRemoteBranchExists(branch.Name)

	if !branchExistsOnRemote {
		return "", errors.New(self.c.Tr.NoBranchOnRemote)
	}

	return self.c.Helpers().Host.GetPullRequestURL(branch.Name, "")
}

func (self *BranchesController) copyPullRequestURL() error {
	url, err := self.getPullRequestURL()
	if err != nil {
		return err
	}
	self.c.LogAction(self.c.Tr.Actions.CopyPullRequestURL)
	if err := self.c.OS().CopyToClipboard(url); err != nil {
		return err
	}

	self.c.Toast(self.c.Tr.PullRequestURLCopiedToClipboard)

	return nil
}

func (self *BranchesController) forceCheckout() error {
	branch := self.context().GetSelected()
	message := self.c.Tr.SureForceCheckout
	title := self.c.Tr.ForceCheckoutBranch

	self.c.Confirm(types.ConfirmOpts{
		Title:  title,
		Prompt: message,
		HandleConfirm: func() error {
			self.c.LogAction(self.c.Tr.Actions.ForceCheckoutBranch)
			if err := self.c.Git().Branch.Checkout(branch.Name, git_commands.CheckoutOptions{Force: true}); err != nil {
				return err
			}
			self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC})
			return nil
		},
	})

	return nil
}

func (self *BranchesController) checkoutPreviousBranch() error {
	self.c.LogAction(self.c.Tr.Actions.CheckoutBranch)
	return self.c.Helpers().Refs.CheckoutPreviousRef()
}

func (self *BranchesController) checkoutByName() error {
	self.c.Prompt(types.PromptOpts{
		Title:               self.c.Tr.BranchName + ":",
		FindSuggestionsFunc: self.c.Helpers().Suggestions.GetRefsSuggestionsFunc(),
		HandleConfirm: func(response string) error {
			self.c.LogAction("Checkout branch")
			_, branchName, found := self.c.Helpers().Refs.ParseRemoteBranchName(response)
			if found {
				return self.c.Helpers().Refs.CheckoutRemoteBranch(response, branchName)
			}
			return self.c.Helpers().Refs.CheckoutRef(response, types.CheckoutRefOptions{
				OnRefNotFound: func(ref string) error {
					self.c.Confirm(types.ConfirmOpts{
						Title:  self.c.Tr.BranchNotFoundTitle,
						Prompt: fmt.Sprintf("%s %s%s", self.c.Tr.BranchNotFoundPrompt, ref, "?"),
						HandleConfirm: func() error {
							return self.createNewBranchWithName(ref)
						},
					})

					return nil
				},
			})
		},
	},
	)

	return nil
}

func (self *BranchesController) createNewBranchWithName(newBranchName string) error {
	branch := self.context().GetSelected()
	if branch == nil {
		return nil
	}

	if err := self.c.Git().Branch.New(newBranchName, branch.FullRefName()); err != nil {
		return err
	}

	self.c.Helpers().Refs.SelectFirstBranchAndFirstCommit()
	self.c.Refresh(types.RefreshOptions{
		Mode:                     types.ASYNC,
		KeepBranchSelectionIndex: true,
		CommitSelection:          types.KeepCommitSelectionIndex,
	})
	return nil
}

func (self *BranchesController) localDelete(branches []*models.Branch) error {
	return self.c.Helpers().BranchesHelper.ConfirmLocalDelete(branches)
}

func (self *BranchesController) remoteDelete(branches []*models.Branch) error {
	remoteBranches := lo.Map(branches, func(branch *models.Branch, _ int) *models.RemoteBranch {
		return &models.RemoteBranch{Name: branch.UpstreamBranch, RemoteName: branch.UpstreamRemote}
	})
	return self.c.Helpers().BranchesHelper.ConfirmDeleteRemote(remoteBranches, false)
}

func (self *BranchesController) localAndRemoteDelete(branches []*models.Branch) error {
	return self.c.Helpers().BranchesHelper.ConfirmLocalAndRemoteDelete(branches)
}

func (self *BranchesController) delete(branches []*models.Branch) error {
	checkedOutBranch := self.c.Helpers().Refs.GetCheckedOutRef()
	isBranchCheckedOut := lo.SomeBy(branches, func(branch *models.Branch) bool {
		return checkedOutBranch.Name == branch.Name
	})
	hasUpstream := lo.EveryBy(branches, func(branch *models.Branch) bool {
		return branch.IsTrackingRemote() && !branch.UpstreamGone
	})

	localDeleteItem := &types.MenuItem{
		Label: lo.Ternary(len(branches) > 1, self.c.Tr.DeleteLocalBranches, self.c.Tr.DeleteLocalBranch),
		Keys:  menuKey('c'),
		OnPress: func() error {
			return self.localDelete(branches)
		},
	}
	if isBranchCheckedOut {
		localDeleteItem.DisabledReason = &types.DisabledReason{Text: self.c.Tr.CantDeleteCheckOutBranch}
	}

	remoteDeleteItem := &types.MenuItem{
		Label: lo.Ternary(len(branches) > 1, self.c.Tr.DeleteRemoteBranches, self.c.Tr.DeleteRemoteBranch),
		Keys:  menuKey('r'),
		OnPress: func() error {
			return self.remoteDelete(branches)
		},
	}
	if !hasUpstream {
		remoteDeleteItem.DisabledReason = &types.DisabledReason{
			Text: lo.Ternary(len(branches) > 1, self.c.Tr.UpstreamsNotSetError, self.c.Tr.UpstreamNotSetError),
		}
	}

	deleteBothItem := &types.MenuItem{
		Label: lo.Ternary(len(branches) > 1, self.c.Tr.DeleteLocalAndRemoteBranches, self.c.Tr.DeleteLocalAndRemoteBranch),
		Keys:  menuKey('b'),
		OnPress: func() error {
			return self.localAndRemoteDelete(branches)
		},
	}
	if isBranchCheckedOut {
		deleteBothItem.DisabledReason = &types.DisabledReason{Text: self.c.Tr.CantDeleteCheckOutBranch}
	} else if !hasUpstream {
		deleteBothItem.DisabledReason = &types.DisabledReason{
			Text: lo.Ternary(len(branches) > 1, self.c.Tr.UpstreamsNotSetError, self.c.Tr.UpstreamNotSetError),
		}
	}

	var menuTitle string
	if len(branches) == 1 {
		menuTitle = utils.ResolvePlaceholderString(
			self.c.Tr.DeleteBranchTitle,
			map[string]string{
				"selectedBranchName": branches[0].Name,
			},
		)
	} else {
		menuTitle = self.c.Tr.DeleteBranchesTitle
	}

	return self.c.Menu(types.CreateMenuOptions{
		Title: menuTitle,
		Items: []*types.MenuItem{localDeleteItem, remoteDeleteItem, deleteBothItem},
	})
}

func (self *BranchesController) merge() error {
	selectedBranchName := self.context().GetSelected().Name
	return self.c.Helpers().MergeAndRebase.MergeRefIntoCheckedOutBranch(selectedBranchName)
}

func (self *BranchesController) rebase(branch *models.Branch) error {
	return self.c.Helpers().MergeAndRebase.RebaseOntoRef(branch.Name)
}

func (self *BranchesController) fastForward(branch *models.Branch) error {
	if !branch.IsTrackingRemote() {
		return errors.New(self.c.Tr.FwdNoUpstream)
	}
	if !branch.RemoteBranchStoredLocally() {
		return errors.New(self.c.Tr.FwdNoLocalUpstream)
	}
	if branch.IsAheadForPull() {
		return errors.New(self.c.Tr.FwdCommitsToPush)
	}

	action := self.c.Tr.Actions.FastForwardBranch

	return self.c.WithInlineStatus(branch, types.ItemOperationFastForwarding, context.LOCAL_BRANCHES_CONTEXT_KEY, func(task gocui.Task) error {
		worktree, ok := self.worktreeForBranch(branch)
		if ok {
			self.c.LogAction(action)

			worktreeGitDir := ""
			worktreePath := ""
			// if it is the current worktree path, no need to specify the path
			if !worktree.IsCurrent {
				worktreeGitDir = worktree.GitDir
				worktreePath = worktree.Path
			}

			err := self.c.Git().Sync.Pull(
				task,
				git_commands.PullOptions{
					RemoteName:      branch.UpstreamRemote,
					BranchName:      branch.UpstreamBranch,
					FastForwardOnly: true,
					WorktreeGitDir:  worktreeGitDir,
					WorktreePath:    worktreePath,
				},
			)
			self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC})
			return err
		}

		self.c.LogAction(action)

		err := self.c.Git().Sync.FastForward(
			task, branch.Name, branch.UpstreamRemote, branch.UpstreamBranch,
		)
		self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC, Scope: []types.RefreshableView{types.BRANCHES}})
		return err
	})
}

func (self *BranchesController) createTag(branch *models.Branch) error {
	return self.c.Helpers().Tags.OpenCreateTagPrompt(branch.FullRefName(), func() {})
}

func (self *BranchesController) createSortMenu() error {
	return self.c.Helpers().Refs.CreateSortOrderMenu(
		[]string{"recency", "alphabetical", "date"},
		self.c.Tr.SortOrderPromptLocalBranches,
		func(sortOrder string) error {
			if self.c.UserConfig().Git.LocalBranchSortOrder != sortOrder {
				self.c.UserConfig().Git.LocalBranchSortOrder = sortOrder
				self.c.Contexts().Branches.SetSelection(0)
				self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC, Scope: []types.RefreshableView{types.BRANCHES}})
				return nil
			}
			return nil
		},
		self.c.UserConfig().Git.LocalBranchSortOrder)
}

func (self *BranchesController) createResetMenu(selectedBranch *models.Branch) error {
	return self.c.Helpers().Refs.CreateGitResetMenu(selectedBranch.Name, selectedBranch.FullRefName())
}

func (self *BranchesController) rename(branch *models.Branch) error {
	promptForNewName := func() error {
		self.c.Prompt(types.PromptOpts{
			Title:          self.c.Tr.NewBranchNamePrompt + " " + branch.Name + ":",
			InitialContent: branch.Name,
			HandleConfirm: func(newBranchName string) error {
				self.c.LogAction(self.c.Tr.Actions.RenameBranch)
				if err := self.c.Git().Branch.Rename(branch.Name, helpers.SanitizedBranchName(newBranchName)); err != nil {
					return err
				}

				// need to find where the branch is now so that we can re-select it. That means we need to refetch the branches synchronously and then find our branch
				self.c.Refresh(types.RefreshOptions{
					Mode:  types.SYNC,
					Scope: []types.RefreshableView{types.BRANCHES, types.WORKTREES},
				})

				// now that we've got our stuff again we need to find that branch and reselect it.
				for i, newBranch := range self.c.Model().Branches {
					if newBranch.Name == newBranchName {
						self.context().SetSelection(i)
						self.context().HandleRender()
					}
				}

				return nil
			},
		})

		return nil
	}

	// I could do an explicit check here for whether the branch is tracking a remote branch
	// but if we've selected it we'll already know that via Pullables and Pullables.
	// Bit of a hack but I'm lazy.
	return self.c.ConfirmIf(branch.IsTrackingRemote(), types.ConfirmOpts{
		Title:         self.c.Tr.RenameBranch,
		Prompt:        self.c.Tr.RenameBranchWarning,
		HandleConfirm: promptForNewName,
	})
}

func (self *BranchesController) newBranch(selectedBranch *models.Branch) error {
	return self.c.Helpers().Refs.NewBranch(selectedBranch.FullRefName(), selectedBranch.RefName(), "")
}

func (self *BranchesController) createPullRequestMenu(selectedBranch *models.Branch, checkedOutBranch *models.Branch) error {
	menuItems := make([]*types.MenuItem, 0, 6)

	// Resolve GitHub auth once — used to decide in-app vs browser for every item.
	svcInfo, token, isGitHub := self.getGitHubAuth()

	fromToLabelColumns := func(from string, to string) []string {
		return []string{fmt.Sprintf("%s → %s", from, to)}
	}

	menuItemsForBranch := func(branch *models.Branch) []*types.MenuItem {
		toDefault := &types.MenuItem{
			LabelColumns: fromToLabelColumns(branch.Name, self.c.Tr.DefaultBranch),
			OnPress: func() error {
				if isGitHub {
					return self.createPRViaAPIWithBase(branch, "", svcInfo, token)
				}
				return self.handleCreatePullRequest(branch)
			},
		}
		toSelect := &types.MenuItem{
			LabelColumns: fromToLabelColumns(branch.Name, self.c.Tr.SelectBranch),
			OnPress: func() error {
				if isGitHub {
					return self.handleCreatePRViaAPI(branch)
				}
				if !branch.IsTrackingRemote() {
					return errors.New(self.c.Tr.PullRequestNoUpstream)
				}
				if len(self.c.Model().Remotes) == 1 {
					toRemote := self.c.Model().Remotes[0].Name
					return self.promptForTargetBranchNameAndCreatePullRequest(branch, toRemote)
				}
				self.c.Prompt(types.PromptOpts{
					Title:               self.c.Tr.SelectTargetRemote,
					FindSuggestionsFunc: self.c.Helpers().Suggestions.GetRemoteSuggestionsFunc(),
					HandleConfirm: func(toRemote string) error {
						return self.promptForTargetBranchNameAndCreatePullRequest(branch, toRemote)
					},
				})
				return nil
			},
		}
		return []*types.MenuItem{toDefault, toSelect}
	}

	if selectedBranch != checkedOutBranch {
		menuItems = append(menuItems,
			&types.MenuItem{
				LabelColumns: fromToLabelColumns(checkedOutBranch.Name, selectedBranch.Name),
				OnPress: func() error {
					if !checkedOutBranch.IsTrackingRemote() || !selectedBranch.IsTrackingRemote() {
						return errors.New(self.c.Tr.PullRequestNoUpstream)
					}
					return self.createPullRequest(checkedOutBranch.UpstreamBranch, selectedBranch.UpstreamBranch)
				},
			},
		)
		menuItems = append(menuItems, menuItemsForBranch(checkedOutBranch)...)
	}

	menuItems = append(menuItems, menuItemsForBranch(selectedBranch)...)

	// Open PR section — only shown when the PR is still OPEN.
	if pr, ok := self.c.Model().PullRequestsMap[selectedBranch.Name]; ok && pr != nil && pr.State == "OPEN" && isGitHub {
		prSection := &types.MenuSection{Title: self.c.Tr.PROpenPullRequests, Column: 0}
		capturedPR := pr
		capturedSvc := svcInfo
		capturedToken := token
		prBase := capturedPR.BaseRefName
		if prBase == "" {
			prBase = self.c.Tr.DefaultBranch
		}
		menuItems = append(menuItems, &types.MenuItem{
			Section:      prSection,
			LabelColumns: []string{fmt.Sprintf("#%d: %s → %s", capturedPR.Number, capturedPR.HeadRefName, prBase)},
			OnPress: func() error {
				return self.handleMergePRViaAPI(capturedPR, capturedSvc, capturedToken)
			},
		})
	}

	return self.c.Menu(types.CreateMenuOptions{Title: fmt.Sprint(self.c.Tr.CreatePullRequestOptions), Items: menuItems})
}

func (self *BranchesController) promptForTargetBranchNameAndCreatePullRequest(fromBranch *models.Branch, toRemote string) error {
	remoteDoesNotExist := lo.NoneBy(self.c.Model().Remotes, func(remote *models.Remote) bool {
		return remote.Name == toRemote
	})
	if remoteDoesNotExist {
		return fmt.Errorf(self.c.Tr.NoValidRemoteName, toRemote)
	}

	self.c.Prompt(types.PromptOpts{
		Title:               fmt.Sprintf("%s → %s/", fromBranch.UpstreamBranch, toRemote),
		FindSuggestionsFunc: self.c.Helpers().Suggestions.GetRemoteBranchesForRemoteSuggestionsFunc(toRemote),
		HandleConfirm: func(toBranch string) error {
			self.c.Log.Debugf("PR will target branch '%s' on remote '%s'", toBranch, toRemote)
			return self.createPullRequest(fromBranch.UpstreamBranch, toBranch)
		},
	})

	return nil
}

// showUnsupportedServiceHelp shows a confirmation dialog explaining how to fix
// an unrecognized remote URL. The confirm button copies the config snippet to
// the clipboard so the user can paste it straight into their config file.
func (self *BranchesController) showUnsupportedServiceHelp() error {
	remoteURL, _ := self.c.Git().Remote.GetRemoteURL("origin")

	// Extract the SSH host alias from the URL (everything between @ and the
	// first : or /).  Falls back to a generic placeholder.
	alias := "your-alias"
	if remoteURL != "" {
		if idx := strings.Index(remoteURL, "@"); idx >= 0 {
			rest := remoteURL[idx+1:]
			if end := strings.IndexAny(rest, ":/"); end >= 0 {
				alias = rest[:end]
			}
		}
	}

	snippet := fmt.Sprintf("services:\n  '%s': 'github:github.com'", alias)

	// Use the real config path that deskgit is actually reading from.
	configPath := filepath.Join(self.c.GetConfig().GetUserConfigDir(), "config.yml")

	urlLine := ""
	if remoteURL != "" {
		urlLine = fmt.Sprintf("\n\nRemote URL: %s", remoteURL)
	}

	prompt := fmt.Sprintf(
		"Remote URL not recognized as a supported git service.%s\n\nAdd to %s:\n\n  services:\n    '%s': 'github:github.com'\n\nPress Enter to copy the snippet — then paste it into your config.",
		urlLine, configPath, alias,
	)

	self.c.Confirm(types.ConfirmOpts{
		Title:  "Unsupported git service — how to fix",
		Prompt: prompt,
		HandleConfirm: func() error {
			if err := self.c.OS().CopyToClipboard(snippet); err != nil {
				return err
			}
			self.c.Toast(fmt.Sprintf("Snippet copied — paste into %s", configPath))
			return nil
		},
	})
	return nil
}

func (self *BranchesController) createPullRequest(from string, to string) error {
	url, err := self.c.Helpers().Host.GetPullRequestURL(from, to)
	if err != nil {
		if err.Error() == self.c.Tr.UnsupportedGitService {
			return self.showUnsupportedServiceHelp()
		}
		return err
	}

	self.c.LogAction(self.c.Tr.Actions.OpenPullRequest)

	if err := self.c.OS().OpenLink(url); err != nil {
		return err
	}

	return nil
}

func (self *BranchesController) branchIsReal(branch *models.Branch) *types.DisabledReason {
	if !branch.IsRealBranch() {
		return &types.DisabledReason{Text: self.c.Tr.SelectedItemIsNotABranch}
	}

	return nil
}

func (self *BranchesController) branchHasPR(branch *models.Branch) *types.DisabledReason {
	if _, ok := self.c.Model().PullRequestsMap[branch.Name]; !ok {
		return &types.DisabledReason{Text: self.c.Tr.NoPullRequestForBranch, ShowErrorInPanel: true}
	}

	return nil
}

func (self *BranchesController) openPRInBrowser(branch *models.Branch) error {
	pr, ok := self.c.Model().PullRequestsMap[branch.Name]
	if !ok {
		// Should be guarded against by the DisabledReason check, but be defensive in case
		// PullRequestsMap was updated concurrently by a background refresh
		return errors.New(self.c.Tr.NoPullRequestForBranch)
	}

	self.c.LogAction(self.c.Tr.Actions.OpenPullRequest)

	return self.c.OS().OpenLink(pr.Url)
}

func (self *BranchesController) branchesAreReal(selectedBranches []*models.Branch, startIdx int, endIdx int) *types.DisabledReason {
	if !lo.EveryBy(selectedBranches, func(branch *models.Branch) bool {
		return branch.IsRealBranch()
	}) {
		return &types.DisabledReason{Text: self.c.Tr.SelectedItemIsNotABranch}
	}

	return nil
}

func (self *BranchesController) notMergingIntoYourself(branch *models.Branch) *types.DisabledReason {
	selectedBranchName := branch.Name
	checkedOutBranch := self.c.Helpers().Refs.GetCheckedOutRef().Name

	if checkedOutBranch == selectedBranchName {
		return &types.DisabledReason{Text: self.c.Tr.CantMergeBranchIntoItself}
	}

	return nil
}
