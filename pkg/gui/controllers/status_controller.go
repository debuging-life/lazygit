package controllers

import (
	"fmt"
	"strings"
	"time"

	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/presentation"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

type StatusController struct {
	baseController
	c *ControllerCommon
}

var _ types.IController = &StatusController{}

func NewStatusController(
	c *ControllerCommon,
) *StatusController {
	return &StatusController{
		baseController: baseController{},
		c:              c,
	}
}

func (self *StatusController) GetKeybindings(opts types.KeybindingsOpts) []*types.Binding {
	bindings := []*types.Binding{
		{
			Keys:            opts.GetKeys(opts.Config.Universal.Edit),
			Handler:         self.editConfig,
			Description:     self.c.Tr.EditConfig,
			Tooltip:         self.c.Tr.EditFileTooltip,
			DisplayOnScreen: true,
		},
		{
			Keys:            opts.GetKeys(opts.Config.Status.CheckForUpdate),
			Handler:         self.handleCheckForUpdate,
			Description:     self.c.Tr.CheckForUpdate,
			DisplayOnScreen: true,
		},
		{
			Keys:            opts.GetKeys(opts.Config.Status.RecentRepos),
			Handler:         self.c.Helpers().Repos.CreateRecentReposMenu,
			Description:     self.c.Tr.SwitchRepo,
			DisplayOnScreen: true,
		},
		{
			Keys:            opts.GetKeys(opts.Config.Status.DesktimersTasks),
			Handler:         self.c.Helpers().Desktimers.OpenTaskMenu,
			Description:     self.c.Tr.DesktimersViewTasks,
			Tooltip:         self.c.Tr.DesktimersTasksTooltip,
			OpensMenu:       true,
			DisplayOnScreen: true,
		},
		{
			Keys:            opts.GetKeys(opts.Config.Status.DesktimersMenu),
			Handler:         self.c.Helpers().Desktimers.OpenDeskTimersMenu,
			Description:     self.c.Tr.DesktimersMenu,
			Tooltip:         self.c.Tr.DesktimersMenuTooltip,
			OpensMenu:       true,
			DisplayOnScreen: true,
		},
		{
			Keys:        opts.GetKeys(opts.Config.Status.AllBranchesLogGraph),
			Handler:     func() error { self.switchToOrRotateAllBranchesLogs(); return nil },
			Description: self.c.Tr.AllBranchesLogGraph,
		},
		{
			Keys:        opts.GetKeys(opts.Config.Status.AllBranchesLogGraphReverse),
			Handler:     func() error { self.switchToOrRotateAllBranchesLogsBackward(); return nil },
			Description: self.c.Tr.AllBranchesLogGraphReverse,
		},
	}

	return bindings
}

func (self *StatusController) GetMouseKeybindings(opts types.KeybindingsOpts) []*gocui.ViewMouseBinding {
	return []*gocui.ViewMouseBinding{
		{
			ViewName: self.Context().GetViewName(),
			Key:      gocui.MouseLeft,
			Handler:  self.onClick,
		},
	}
}

func (self *StatusController) GetOnRenderToMain() func() {
	return func() {
		switch self.c.UserConfig().Gui.StatusPanelView {
		case "dashboard":
			self.showDashboard()
		case "allBranchesLog":
			self.showAllBranchLogs()
		default:
			self.showDashboard()
		}
	}
}

func (self *StatusController) Context() types.Context {
	return self.c.Contexts().Status
}

func (self *StatusController) onClick(opts gocui.ViewMouseBindingOpts) error {
	// TODO: move into some abstraction (status is currently not a listViewContext where a lot of this code lives)
	currentBranch := self.c.Helpers().Refs.GetCheckedOutRef()
	if currentBranch == nil {
		// need to wait for branches to refresh
		return nil
	}

	self.c.Context().Push(self.Context(), types.OnFocusOpts{})

	upstreamStatus := utils.Decolorise(presentation.BranchStatus(currentBranch, types.ItemOperationNone, self.c.Tr, time.Now(), self.c.UserConfig()))
	repoName := self.c.Git().RepoPaths.RepoName()
	workingTreeState := self.c.Git().Status.WorkingTreeState()
	if workingTreeState.Any() {
		workingTreeStatus := fmt.Sprintf("(%s)", workingTreeState.LowerCaseTitle(self.c.Tr))
		if cursorInSubstring(opts.X, upstreamStatus+" ", workingTreeStatus) {
			return self.c.Helpers().MergeAndRebase.CreateRebaseOptionsMenu()
		}
		if cursorInSubstring(opts.X, upstreamStatus+" "+workingTreeStatus+" ", repoName) {
			return self.c.Helpers().Repos.CreateRecentReposMenu()
		}
	} else if cursorInSubstring(opts.X, upstreamStatus+" ", repoName) {
		return self.c.Helpers().Repos.CreateRecentReposMenu()
	}

	return nil
}

func runeCount(str string) int {
	return len([]rune(str))
}

func cursorInSubstring(cx int, prefix string, substring string) bool {
	return cx >= runeCount(prefix) && cx < runeCount(prefix+substring)
}

func deskgitTitle() string {
	return `
      _           _         _ _
   __| | ___  ___| | ____ _(_) |_
  / _` + "`" + ` |/ _ \/ __| |/ / _` + "`" + ` | | __|
 | (_| |  __/\__ \   < (_| | | |_
  \__,_|\___||___/_|\_\__, |_|\__|
                      |___/       `
}

func (self *StatusController) editConfig() error {
	return (&EditConfigAction{c: self.c}).Call()
}

func (self *StatusController) showAllBranchLogs() {
	cmdObj := self.c.Git().Branch.AllBranchesLogCmdObj()
	task := types.NewRunPtyTask(cmdObj.GetCmd())

	title := self.c.Tr.LogTitle
	if i, n := self.c.Git().Branch.GetAllBranchesLogIdxAndCount(); n > 1 {
		title = fmt.Sprintf(self.c.Tr.LogXOfYTitle, i+1, n)
	}
	self.c.RenderToMainViews(types.RefreshMainOpts{
		Pair: self.c.MainViewPairs().Normal,
		Main: &types.ViewUpdateOpts{
			Title: title,
			Task:  task,
		},
	})
}

// Switches to the all branches view, or, if already on that view,
// rotates to the next command in the list, and then renders it.
func (self *StatusController) switchToOrRotateAllBranchesLogs() {
	// A bit of a hack to ensure we only rotate to the next branch log command
	// if we currently are looking at a branch log. Otherwise, we should just show
	// the current index (if we are coming from the dashboard).
	if self.c.Views().Main.Title != self.c.Tr.StatusTitle {
		self.c.Git().Branch.RotateAllBranchesLogIdx()
	}
	self.showAllBranchLogs()
}

// Switches to the all branches view, or, if already on that view,
// rotates to the previous command in the list, and then renders it.
func (self *StatusController) switchToOrRotateAllBranchesLogsBackward() {
	// A bit of a hack to ensure we only rotate to the previous branch log command
	// if we currently are looking at a branch log. Otherwise, we should just show
	// the current index (if we are coming from the dashboard).
	if self.c.Views().Main.Title != self.c.Tr.StatusTitle {
		self.c.Git().Branch.RotateAllBranchesLogIdxBackward()
	}
	self.showAllBranchLogs()
}

func (self *StatusController) showDashboard() {
	dashboardString := strings.Join(
		[]string{
			deskgitTitle(),
			"DeskTimers — task-bound git client\nPick your task; every branch, commit, and push maps back to it in DeskTimers.",
			"Website: https://www.desktimers.com",
			"deskgit repo: https://github.com/debuging-life/lazygit",
			"Install: brew install debuging-life/tap/deskgit — https://github.com/debuging-life/homebrew-tap",
			"Keybindings: https://github.com/debuging-life/lazygit/blob/main/docs/keybindings/Keybindings_en.md",
			"Config Options: https://github.com/debuging-life/lazygit/blob/main/docs/Config.md",
			"Raise an issue: https://github.com/debuging-life/lazygit/issues",
			"Based on lazygit by Jesse Duffield (MIT): https://github.com/jesseduffield/lazygit",
			fmt.Sprintf("Copyright %d LoudOwls · lazygit Copyright Jesse Duffield", time.Now().Year()),
		}, "\n\n") + "\n"

	self.c.RenderToMainViews(types.RefreshMainOpts{
		Pair: self.c.MainViewPairs().Normal,
		Main: &types.ViewUpdateOpts{
			Title: self.c.Tr.StatusTitle,
			Task:  types.NewRenderStringTask(dashboardString),
		},
	})
}

func (self *StatusController) handleCheckForUpdate() error {
	return self.c.Helpers().Update.CheckForUpdateInForeground()
}
