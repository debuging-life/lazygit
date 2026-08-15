package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jesseduffield/lazygit/pkg/config"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"github.com/samber/lo"
)

// DesktimersHelper owns the deskgit-specific UI: the task picker, the
// status-line segment, branch-name prefixes, and the hook install prompt.
type DesktimersHelper struct {
	c *HelperCommon
	// running deskgit version, for the on-demand update check.
	version string

	// cached selected-task state so the status line doesn't hit the disk on
	// every layout pass. nil = not loaded yet or no selection.
	mutex        sync.Mutex
	cachedState  *desktimers.State
	stateLoaded  bool
	hooksChecked bool
	// cached "is there a usable token" answer, same reason as above.
	authChecked   bool
	authenticated bool

	// code of the most-recently created task this session, so the picker can
	// float it to the top under a "Just created" section header.
	recentlyCreatedCode string
}

func NewDesktimersHelper(c *HelperCommon, version string) *DesktimersHelper {
	return &DesktimersHelper{c: c, version: version}
}

// SelectedTask returns the task selected for the current worktree, or nil.
func (self *DesktimersHelper) SelectedTask() *desktimers.State {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	if !self.stateLoaded {
		state, err := desktimers.LoadState(".")
		if err == nil {
			self.cachedState = state
		}
		self.stateLoaded = true
	}
	return self.cachedState
}

func (self *DesktimersHelper) setSelectedTask(state *desktimers.State) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.cachedState = state
	self.stateLoaded = true
}

// StatusLineSegment renders the selected task for the information line.
func (self *DesktimersHelper) StatusLineSegment(maxWidth int) string {
	return desktimers.StatusLineSegment(self.SelectedTask(), maxWidth)
}

// StatusLineHint renders the "no task (alt+t)" nudge: only when the user is
// authenticated and no task is selected.
func (self *DesktimersHelper) StatusLineHint(maxWidth int) string {
	if self.SelectedTask() != nil || !self.isAuthenticated() {
		return ""
	}
	keyLabel := desktimers.FriendlyKeyLabel(self.c.UserConfig().Keybinding.Universal.DesktimersTasks)
	return desktimers.StatusLineHint(keyLabel, maxWidth)
}

func (self *DesktimersHelper) isAuthenticated() bool {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	if !self.authChecked {
		token, err := desktimers.LoadToken()
		self.authenticated = err == nil && token.Valid()
		self.authChecked = true
	}
	return self.authenticated
}

func (self *DesktimersHelper) setLoggedOut() {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.authenticated = false
	self.authChecked = true
}

// openCreateTaskFlow is the "＋ Create task…" flow: project menu → title
// prompt → CreateTask → select the new task exactly like a normal pick
// (SaveState, task_selected report, status line), then hand it to onCreated
// so an interrupted commit/branch flow continues seamlessly. API errors show
// a toast and reopen the originating picker (reopenPicker).
func (self *DesktimersHelper) openCreateTaskFlow(onCreated func(desktimers.Task) error, reopenPicker func() error) error {
	var projects []desktimers.Project
	err := self.c.WithWaitingStatusSync(self.c.Tr.DesktimersLoadingProjects, func() error {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			return err
		}
		projects, err = client.GetProjects()
		return err
	})
	if err != nil {
		self.c.ErrorToast(err.Error())
		return reopenPicker()
	}
	if len(projects) == 0 {
		self.c.ErrorToast(self.c.Tr.DesktimersNoProjects)
		return reopenPicker()
	}

	multiWorkspace := desktimers.ProjectsSpanWorkspaces(projects)
	menuItems := lo.Map(projects, func(project desktimers.Project, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: desktimers.ProjectMenuColumns(project, multiWorkspace),
			OnPress: func() error {
				return self.promptNewTaskTitle(project, onCreated, reopenPicker)
			},
		}
	})

	return self.c.Menu(types.CreateMenuOptions{
		Title: self.c.Tr.DesktimersPickProjectTitle,
		Items: menuItems,
	})
}

func (self *DesktimersHelper) promptNewTaskTitle(project desktimers.Project, onCreated func(desktimers.Task) error, reopenPicker func() error) error {
	self.c.Prompt(types.PromptOpts{
		Title: utils.ResolvePlaceholderString(
			self.c.Tr.DesktimersNewTaskTitlePrompt,
			map[string]string{"project": project.Name},
		),
		HandleConfirm: func(title string) error {
			title = strings.TrimSpace(title)
			if title == "" {
				self.c.ErrorToast(self.c.Tr.DesktimersTaskTitleEmpty)
				return self.promptNewTaskTitle(project, onCreated, reopenPicker)
			}

			var task desktimers.Task
			err := self.c.WithWaitingStatusSync(self.c.Tr.DesktimersCreatingTask, func() error {
				client, err := desktimers.NewClientFromToken()
				if err != nil {
					return err
				}
				task, err = client.CreateTask(project.ID, title)
				return err
			})
			if err != nil {
				self.c.ErrorToast(err.Error())
				return reopenPicker()
			}

			self.recentlyCreatedCode = task.Code
			if err := self.selectTask(task); err != nil {
				return err
			}
			return onCreated(task)
		},
	})
	return nil
}

// timerSpinnerFrame returns a spinning character that cycles each second,
// giving an animated feel each time the menu is (re-)opened.
func timerSpinnerFrame() string {
	frames := []string{"◐", "◓", "◑", "◒"}
	return frames[time.Now().Unix()%int64(len(frames))]
}

// styledTaskColumns applies ANSI color to a task's label columns.
// Tracking tasks are rendered yellow+bold with an animated spinner;
// in-progress tasks are rendered in cyan.
func styledTaskColumns(columns []string, task desktimers.Task) []string {
	if len(columns) == 0 {
		return columns
	}
	out := make([]string, len(columns))
	copy(out, columns)
	switch {
	case task.Tracking:
		// Animate the marker column (index 3) with a spinner.
		if len(out) >= 4 {
			out[3] = timerSpinnerFrame() + " " + out[3]
		}
		for i, c := range out {
			out[i] = style.FgYellow.SetBold().Sprint(c)
		}
	case task.Status == desktimers.StatusInProgress:
		for i, c := range out {
			out[i] = style.FgCyan.Sprint(c)
		}
	}
	return out
}

// buildTaskMenuItems converts an ordered task list into menu items, floating
// the recently-created task (if any) to the top under its own section header.
// makeItem receives the task and must return the OnPress/OnOpen/OnInfo callbacks
// appropriate for the caller (OpenTaskMenu vs PickTaskForAction).
func (self *DesktimersHelper) buildTaskMenuItems(
	ordered []desktimers.Task,
	makeItem func(desktimers.Task) *types.MenuItem,
) []*types.MenuItem {
	recentCode := self.recentlyCreatedCode

	justCreatedSection := &types.MenuSection{Title: self.c.Tr.DesktimersJustCreated, Column: 0}
	activeSection := &types.MenuSection{Title: "", Column: 0}

	items := make([]*types.MenuItem, 0, len(ordered))

	// Recently-created task pinned at top (separate section).
	for _, t := range ordered {
		if recentCode != "" && t.Code == recentCode {
			item := makeItem(t)
			item.LabelColumns = styledTaskColumns(item.LabelColumns, t)
			item.Section = justCreatedSection
			items = append(items, item)
			break
		}
	}

	// All other tasks.
	useSections := recentCode != ""
	for _, t := range ordered {
		if recentCode != "" && t.Code == recentCode {
			continue
		}
		item := makeItem(t)
		item.LabelColumns = styledTaskColumns(item.LabelColumns, t)
		if useSections {
			item.Section = activeSection
		}
		items = append(items, item)
	}

	return items
}

// createTaskMenuItem builds the shared "＋ Create task…" picker item.
func (self *DesktimersHelper) createTaskMenuItem(onCreated func(desktimers.Task) error, reopenPicker func() error) *types.MenuItem {
	return &types.MenuItem{
		Label: self.c.Tr.DesktimersCreateTask,
		OnPress: func() error {
			return self.openCreateTaskFlow(onCreated, reopenPicker)
		},
		OpensMenu: true,
	}
}

func (self *DesktimersHelper) clearSelectedTask() error {
	if err := desktimers.ClearState("."); err != nil {
		return err
	}
	self.setSelectedTask(nil)
	return nil
}

// OpenDeskTimersMenu is the alt+d launcher: one menu dispatching to every
// DeskTimers action, so nobody has to memorize the individual bindings.
func (self *DesktimersHelper) OpenDeskTimersMenu() error {
	state := self.SelectedTask()
	var noTaskReason *types.DisabledReason
	stateURL := ""
	if state == nil {
		noTaskReason = &types.DisabledReason{Text: self.c.Tr.DesktimersNoTaskSelected}
	} else {
		stateURL = state.URL
	}

	menuItems := []*types.MenuItem{
		{
			Label:     self.c.Tr.DesktimersViewPickTask,
			OnPress:   self.OpenTaskMenu,
			Keys:      menuKey('t'),
			OpensMenu: true,
		},
		{
			Label: self.c.Tr.DesktimersOpenTaskInBrowser,
			OnPress: func() error {
				return self.openTaskInBrowser(stateURL)
			},
			Keys:           menuKey('o'),
			DisabledReason: noTaskReason,
		},
		{
			Label:          self.c.Tr.DesktimersClearTask,
			OnPress:        self.clearSelectedTask,
			Keys:           menuKey('c'),
			DisabledReason: noTaskReason,
		},
		{
			Label:   self.c.Tr.DesktimersSetupRepo,
			OnPress: self.setupRepositoryRemote,
			Keys:    menuKey('s'),
		},
		{
			Label:   self.c.Tr.DesktimersCheckForUpdatesNow,
			OnPress: self.checkForUpdatesNow,
			Keys:    menuKey('u'),
		},
		{
			Label:   self.c.Tr.DesktimersLogout,
			OnPress: self.confirmLogout,
			Keys:    menuKey('l'),
		},
	}

	return self.c.Menu(types.CreateMenuOptions{
		Title: self.c.Tr.DesktimersMenuTitle,
		Items: menuItems,
	})
}

// checkForUpdatesNow runs the tap version check bypassing the 24h cache and
// reports the outcome in a popup.
func (self *DesktimersHelper) checkForUpdatesNow() error {
	var latest string
	err := self.c.WithWaitingStatusSync(self.c.Tr.DesktimersCheckingForUpdates, func() error {
		var err error
		latest, err = desktimers.LatestReleasedVersion(true)
		return err
	})
	if err != nil {
		return err
	}

	if desktimers.IsNewerVersion(latest, self.version) {
		return self.showUpdateAvailable(latest)
	} else {
		self.c.Alert(self.c.Tr.DesktimersCheckForUpdatesNow, utils.ResolvePlaceholderString(
			self.c.Tr.DesktimersUpToDate,
			map[string]string{"version": latest},
		))
	}
	return nil
}

// openTaskInBrowser opens a task's webapp deep link; a missing link shows a
// status toast instead of erroring.
func (self *DesktimersHelper) openTaskInBrowser(url string) error {
	if url == "" {
		self.c.ErrorToast(self.c.Tr.DesktimersNoTaskURL)
		return nil
	}
	return self.c.OS().OpenLink(url)
}

func (self *DesktimersHelper) showTaskInfo(task desktimers.Task) error {
	status := task.Status
	if status == "" {
		status = "—"
	}
	prompt := utils.ResolvePlaceholderString(
		self.c.Tr.DesktimersTaskInfoPrompt,
		map[string]string{
			"code":    task.Code,
			"title":   task.Title,
			"project": task.Project,
			"status":  status,
		},
	)
	self.c.Alert(self.c.Tr.DesktimersTaskInfoTitle, prompt)
	return nil
}

func (self *DesktimersHelper) resetAuthCache() {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.authChecked = false
}

// promptReauthThen handles an expired login inside the TUI: confirm, then
// run `deskgit login` (the terminal device flow) as a subprocess with the
// gui suspended, and on success resume the original action.
func (self *DesktimersHelper) promptReauthThen(retry func() error) error {
	self.c.Confirm(types.ConfirmOpts{
		Title:  self.c.Tr.DesktimersLoginExpiredTitle,
		Prompt: self.c.Tr.DesktimersLoginExpiredPrompt,
		HandleConfirm: func() error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if err := self.c.RunSubprocessAndRefresh(self.c.OS().Cmd.New([]string{exe, "login"})); err != nil {
				return err
			}
			self.resetAuthCache()

			// Only resume when the login actually produced a valid token
			// (the user may have Ctrl-C'd the terminal flow).
			if token, err := desktimers.LoadToken(); err != nil || !token.Valid() {
				return nil
			}
			return retry()
		},
	})
	return nil
}

// BranchNamePrefix returns "CODE/" when a task is selected, else "".
func (self *DesktimersHelper) BranchNamePrefix() string {
	state := self.SelectedTask()
	if state == nil || state.Code == "" {
		return ""
	}
	return desktimers.BranchPrefix(state.Code)
}

// OpenTaskMenu fetches the user's tasks and shows the picker menu.
func (self *DesktimersHelper) OpenTaskMenu() error {
	desktimers.SetConfiguredBaseURL(self.c.UserConfig().Desktimers.ApiBaseUrl)

	var tasks []desktimers.Task
	err := self.c.WithWaitingStatusSync(self.c.Tr.DesktimersLoadingTasks, func() error {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			return err
		}
		tasks, err = client.GetTasks("active")
		return err
	})
	if err != nil {
		if errors.Is(err, desktimers.ErrUnauthorized) {
			return self.promptReauthThen(self.OpenTaskMenu)
		}
		return err
	}

	currentCode := ""
	if state := self.SelectedTask(); state != nil {
		currentCode = state.Code
	}
	ordered, selectedIdx := desktimers.OrderTasksForPicker(tasks, currentCode)

	menuItems := self.buildTaskMenuItems(ordered, func(task desktimers.Task) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: desktimers.PickerColumns(task),
			OnPress: func() error {
				return self.selectTask(task)
			},
			OnOpen: func() error {
				return self.openTaskInBrowser(task.URL)
			},
			OnInfo: func() error {
				return self.showTaskInfo(task)
			},
		}
	})

	if len(menuItems) == 0 {
		menuItems = append(menuItems, &types.MenuItem{
			Label:          self.c.Tr.DesktimersNoTasks,
			OnPress:        func() error { return nil },
			DisabledReason: &types.DisabledReason{Text: self.c.Tr.DesktimersNoTasks},
		})
	}

	if state := self.SelectedTask(); state != nil {
		stateURL := state.URL
		menuItems = append(menuItems,
			&types.MenuItem{
				Label: self.c.Tr.DesktimersOpenTaskInBrowser,
				OnPress: func() error {
					return self.openTaskInBrowser(stateURL)
				},
			},
			&types.MenuItem{
				Label:   self.c.Tr.DesktimersClearTask,
				OnPress: self.clearSelectedTask,
			})
	}

	menuItems = append(menuItems,
		self.createTaskMenuItem(func(desktimers.Task) error { return nil }, self.OpenTaskMenu),
		&types.MenuItem{
			Label:   self.c.Tr.DesktimersLogout,
			OnPress: self.confirmLogout,
		})

	return self.c.Menu(types.CreateMenuOptions{
		Title:              self.c.Tr.DesktimersTaskMenuTitle,
		Items:              menuItems,
		InitialSelectedIdx: selectedIdx,
		MinWidth:           120,
	})
}

// confirmLogout asks for confirmation, then revokes the device token and
// clears the local login. The TUI keeps running for plain git use.
func (self *DesktimersHelper) confirmLogout() error {
	self.c.Confirm(types.ConfirmOpts{
		Title:  self.c.Tr.DesktimersLogout,
		Prompt: self.c.Tr.DesktimersLogoutConfirm,
		HandleConfirm: func() error {
			outcome, err := desktimers.Logout()
			if err != nil {
				return err
			}
			self.setLoggedOut()
			message := self.c.Tr.DesktimersLoggedOut
			if outcome.Result == desktimers.LogoutLocalOnly {
				message = self.c.Tr.DesktimersLoggedOutOffline
			}
			self.c.Alert(self.c.Tr.DesktimersLogout, message)
			return nil
		},
	})
	return nil
}

// PickTaskForAction shows the task picker as a mandatory step inside another
// flow (commit, new branch): tasks are fetched LIVE, in-progress tasks float
// to the top, and the currently-selected task is preselected. Picking
// persists the selection (keeping the hooks working for outside-deskgit
// commits) and then calls onPick. Escape/cancel closes the menu without
// calling onPick, aborting the surrounding flow.
func (self *DesktimersHelper) PickTaskForAction(onPick func(desktimers.Task) error) error {
	desktimers.SetConfiguredBaseURL(self.c.UserConfig().Desktimers.ApiBaseUrl)

	var tasks []desktimers.Task
	err := self.c.WithWaitingStatusSync(self.c.Tr.DesktimersLoadingTasks, func() error {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			return err
		}
		tasks, err = client.GetTasks("active")
		return err
	})
	if err != nil {
		if errors.Is(err, desktimers.ErrUnauthorized) {
			return self.promptReauthThen(func() error { return self.PickTaskForAction(onPick) })
		}
		return err
	}

	currentCode := ""
	if state := self.SelectedTask(); state != nil {
		currentCode = state.Code
	}
	ordered, selectedIdx := desktimers.OrderTasksForPicker(tasks, currentCode)

	menuItems := self.buildTaskMenuItems(ordered, func(task desktimers.Task) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: desktimers.PickerColumns(task),
			OnPress: func() error {
				if err := self.selectTask(task); err != nil {
					return err
				}
				return onPick(task)
			},
			OnOpen: func() error {
				return self.openTaskInBrowser(task.URL)
			},
			OnInfo: func() error {
				return self.showTaskInfo(task)
			},
		}
	})

	if len(menuItems) == 0 {
		// Safety valve: a user with no assigned tasks must still be able to
		// commit; the action proceeds without a code.
		menuItems = append(menuItems, &types.MenuItem{
			Label: self.c.Tr.DesktimersContinueWithoutTask,
			OnPress: func() error {
				return onPick(desktimers.Task{})
			},
		})
	}

	// Creating a task mid-flow selects it and continues the interrupted
	// commit/branch action with the fresh code.
	menuItems = append(menuItems, self.createTaskMenuItem(onPick, func() error {
		return self.PickTaskForAction(onPick)
	}))

	return self.c.Menu(types.CreateMenuOptions{
		Title:              self.c.Tr.DesktimersPickTaskTitle,
		Items:              menuItems,
		InitialSelectedIdx: selectedIdx,
		MinWidth:           120,
	})
}

// CheckForUpdateInBackground checks the Homebrew tap for a newer deskgit
// release (cache-limited to once per 24h) and shows a one-time notice.
// Dev builds (non-semver versions) never nag.
func (self *DesktimersHelper) CheckForUpdateInBackground(currentVersion string) {
	if !self.c.UserConfig().Desktimers.CheckForUpdates {
		return
	}
	log := self.c.Log
	go func() {
		latest, newer, err := desktimers.CheckForUpdate(currentVersion)
		if err != nil {
			log.Debugf("deskgit: update check failed: %v", err)
			return
		}
		if !newer {
			return
		}
		self.c.OnUIThread(func() error {
			return self.showUpdateAvailable(latest)
		})
	}()
}

// RefreshWorkReposInBackground refreshes the server-provided work-repo list
// (used by auto hook installation) when the 24h cache is stale.
func (self *DesktimersHelper) RefreshWorkReposInBackground() {
	if _, fresh := desktimers.CachedWorkRepos(); fresh {
		return
	}
	log := self.c.Log
	go func() {
		if err := desktimers.RefreshWorkRepos(); err != nil {
			log.Debugf("deskgit: work-repo refresh failed: %v", err)
		}
	}()
}

// showUpdateAvailable offers "Update now / Later" for Homebrew installs
// (binary under the Cellar); other installs get the instruction-only alert.
func (self *DesktimersHelper) showUpdateAvailable(latest string) error {
	message := utils.ResolvePlaceholderString(
		self.c.Tr.DesktimersUpdateAvailable,
		map[string]string{"version": latest},
	)

	exe, err := os.Executable()
	if err != nil || !strings.Contains(exe, "/Cellar/") {
		self.c.Alert(self.c.Tr.DesktimersUpdateAvailableTitle, message)
		return nil
	}

	return self.c.Menu(types.CreateMenuOptions{
		Title:  self.c.Tr.DesktimersUpdateAvailableTitle,
		Prompt: message,
		Items: []*types.MenuItem{
			{
				Label:   self.c.Tr.DesktimersUpdateNow,
				OnPress: self.runBrewUpgrade,
			},
			{
				Label:   self.c.Tr.DesktimersUpdateLater,
				OnPress: func() error { return nil },
			},
		},
	})
}

// runBrewUpgrade upgrades deskgit via Homebrew with the gui suspended (same
// mechanism as re-auth), then asks for a restart.
func (self *DesktimersHelper) runBrewUpgrade() error {
	if err := self.c.RunSubprocessAndRefresh(self.c.OS().Cmd.New([]string{"brew", "upgrade", "debuging-life/tap/deskgit"})); err != nil {
		return err
	}
	self.c.Alert(self.c.Tr.DesktimersUpdateAvailableTitle, self.c.Tr.DesktimersRestartAfterUpdate)
	return nil
}

// PickBranchTypeThen shows the branch-type menu (feature/bugfix/hotfix/...)
// and calls then with the chosen type. The first configured type is
// preselected; Escape/cancel closes the menu without calling then, aborting
// the surrounding flow.
func (self *DesktimersHelper) PickBranchTypeThen(branchTypes []string, then func(branchType string) error) error {
	if len(branchTypes) == 0 {
		branchTypes = desktimers.DefaultBranchTypes
	}

	menuItems := lo.Map(branchTypes, func(branchType string, _ int) *types.MenuItem {
		return &types.MenuItem{
			Label: branchType,
			OnPress: func() error {
				return then(branchType)
			},
		}
	})

	return self.c.Menu(types.CreateMenuOptions{
		Title: self.c.Tr.DesktimersBranchTypeTitle,
		Items: menuItems,
	})
}

func (self *DesktimersHelper) selectTask(task desktimers.Task) error {
	previousCode := ""
	if prev := self.SelectedTask(); prev != nil {
		previousCode = prev.Code
	}

	state := desktimers.NewState(task)
	if err := desktimers.SaveState(".", state); err != nil {
		return err
	}
	self.setSelectedTask(state)

	// Only a CHANGE of task is reported — re-picking the same task isn't.
	if task.Code != previousCode {
		self.reportTaskSelectedInBackground(task.Code)
	}
	return nil
}

// reportTaskSelectedInBackground fires the task_selected event to
// DeskTimers: fire-and-forget, bounded by the client's short report
// timeout, failures only ever logged — never surfaced, never blocking.
func (self *DesktimersHelper) reportTaskSelectedInBackground(code string) {
	log := self.c.Log
	go func() {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			log.Debugf("deskgit: skipping task_selected report: %v", err)
			return
		}
		repo := desktimers.RepoSlug(".")
		branch := desktimers.CurrentBranch(".")
		if err := client.ReportTaskSelected(code, repo, branch); err != nil {
			log.Warnf("deskgit: failed to report task selection: %v", err)
		}
	}()
}

// autoInstallHooksForWorkRepo is autoInstallHooks 'auto' mode: repos whose
// origin remote belongs to a configured work org get hooks installed (or
// refreshed) silently and their strictpush config synced; any other repo —
// including ones with no origin remote — is left completely alone. The
// prompt-mode "declined" marker is deliberately ignored here. Everything is
// log-only; no dialogs, no toasts.
func (self *DesktimersHelper) autoInstallHooksForWorkRepo(cfg config.DesktimersConfig) {
	slug := desktimers.RepoSlug(".")
	workRepos, _ := desktimers.CachedWorkRepos()
	if !desktimers.RepoIsWork(slug, workRepos, cfg.HookOrgs) {
		self.c.Log.Debugf("deskgit: repo %q is not a known work repo — leaving it alone", slug)
		return
	}

	if err := desktimers.SyncStrictPushConfig(".", cfg.StrictPush); err != nil {
		self.c.Log.Debugf("deskgit: could not sync strictpush config: %v", err)
	}

	status, err := desktimers.HooksStatus(".")
	if err != nil {
		var customPathErr *desktimers.ErrCustomHooksPath
		if errors.As(err, &customPathErr) {
			self.c.Log.Infof("deskgit: %q uses a custom core.hooksPath — hooks not installed", slug)
		} else {
			self.c.Log.Debugf("deskgit: hooks status check failed: %v", err)
		}
		return
	}
	if status == desktimers.HooksInstalled {
		return
	}

	dtHookPath, ok := desktimers.FindDtHookBinary()
	if !ok {
		self.c.Log.Warnf("deskgit: dt-hook binary not found — cannot install hooks in %q", slug)
		return
	}

	if err := desktimers.InstallHooks(".", dtHookPath); err != nil {
		self.c.Log.Warnf("deskgit: installing hooks in %q failed: %v", slug, err)
		return
	}
	self.c.Log.Infof("deskgit: installed git hooks in %q (work org)", slug)
}

// declinedMarkerPath records that the user declined hook installation for
// this repo, so we only ask once.
func declinedMarkerPath() (string, error) {
	dir, err := desktimers.ConfigDir()
	if err != nil {
		return "", err
	}
	repoPath, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hooks-declined", desktimers.PathKey(repoPath)), nil
}

// PromptToInstallHooksInBackground checks hook status once per repo session
// and, depending on desktimers.autoInstallHooks, installs silently (auto in
// work-org repos, always) or prompts.
func (self *DesktimersHelper) PromptToInstallHooksInBackground() {
	self.mutex.Lock()
	alreadyChecked := self.hooksChecked
	self.hooksChecked = true
	self.mutex.Unlock()
	if alreadyChecked {
		return
	}

	cfg := self.c.UserConfig().Desktimers
	if cfg.AutoInstallHooks == "auto" || cfg.AutoInstallHooks == "" {
		self.autoInstallHooksForWorkRepo(cfg)
		return
	}

	// Legacy modes (prompt/always/never): sync strictpush unconditionally,
	// as before.
	if err := desktimers.SyncStrictPushConfig(".", cfg.StrictPush); err != nil {
		self.c.Log.Debugf("deskgit: could not sync strictpush config: %v", err)
	}

	mode := cfg.AutoInstallHooks
	if mode == "never" {
		return
	}

	status, err := desktimers.HooksStatus(".")
	if err != nil {
		var customPathErr *desktimers.ErrCustomHooksPath
		if errors.As(err, &customPathErr) {
			self.c.OnUIThread(func() error {
				self.c.ErrorToast(self.c.Tr.DesktimersCustomHooksPathNotice)
				return nil
			})
		}
		return
	}
	if status == desktimers.HooksInstalled {
		return
	}

	dtHookPath, ok := desktimers.FindDtHookBinary()
	if !ok {
		// Without the binary an installed hook would be inert; skip quietly.
		return
	}

	if mode == "always" {
		_ = desktimers.InstallHooks(".", dtHookPath)
		return
	}

	// mode == "prompt"
	if marker, err := declinedMarkerPath(); err == nil {
		if _, err := os.Stat(marker); err == nil {
			return
		}
	}

	self.c.OnUIThread(func() error {
		self.c.Confirm(types.ConfirmOpts{
			Title:  self.c.Tr.DesktimersInstallHooksTitle,
			Prompt: self.c.Tr.DesktimersInstallHooksPrompt,
			HandleConfirm: func() error {
				return desktimers.InstallHooks(".", dtHookPath)
			},
			HandleClose: func() error {
				if marker, err := declinedMarkerPath(); err == nil {
					_ = os.MkdirAll(filepath.Dir(marker), 0o700)
					_ = os.WriteFile(marker, []byte("declined\n"), 0o600)
				}
				return nil
			},
		})
		return nil
	})
}

// setupRepositoryRemote lets the user add a new remote or update an existing
// one's URL — from the DeskTimers menu.
func (self *DesktimersHelper) setupRepositoryRemote() error {
	remotes, err := self.c.Git().Loaders.RemoteLoader.GetRemotes()
	if err != nil {
		return err
	}

	if len(remotes) == 0 {
		return self.promptAddRemote("")
	}

	// Build menu: one item per existing remote + "Add new remote"
	items := make([]*types.MenuItem, 0, len(remotes)+1)
	for _, r := range remotes {
		r := r
		displayURL := ""
		if len(r.Urls) > 0 {
			displayURL = r.Urls[0]
		}
		items = append(items, &types.MenuItem{
			LabelColumns: []string{r.Name, displayURL},
			OnPress: func() error {
				return self.promptUpdateRemote(r.Name, displayURL)
			},
		})
	}
	items = append(items, &types.MenuItem{
		Label: self.c.Tr.DesktimersSetupRepoAddNew,
		OnPress: func() error {
			return self.promptAddRemote("")
		},
	})

	return self.c.Menu(types.CreateMenuOptions{
		Title: self.c.Tr.DesktimersSetupRepoSelectRemote,
		Items: items,
	})
}

func (self *DesktimersHelper) promptAddRemote(initialName string) error {
	self.c.Prompt(types.PromptOpts{
		Title:          self.c.Tr.DesktimersSetupRepoNamePrompt,
		InitialContent: initialName,
		HandleConfirm: func(name string) error {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			self.c.Prompt(types.PromptOpts{
				Title: self.c.Tr.DesktimersSetupRepoURLPrompt,
				HandleConfirm: func(url string) error {
					url = strings.TrimSpace(url)
					if url == "" {
						return nil
					}
					if err := self.c.Git().Remote.AddRemote(name, url); err != nil {
						return err
					}
					self.c.Toast(fmt.Sprintf(self.c.Tr.DesktimersSetupRepoAdded, name))
					self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC, Scope: []types.RefreshableView{types.REMOTES}})
					return nil
				},
			})
			return nil
		},
	})
	return nil
}

func (self *DesktimersHelper) promptUpdateRemote(name, currentURL string) error {
	self.c.Prompt(types.PromptOpts{
		Title:          fmt.Sprintf(self.c.Tr.DesktimersSetupRepoURLUpdatePrompt, name),
		InitialContent: currentURL,
		HandleConfirm: func(url string) error {
			url = strings.TrimSpace(url)
			if url == "" || url == currentURL {
				return nil
			}
			if err := self.c.Git().Remote.UpdateRemoteUrl(name, url); err != nil {
				return err
			}
			self.c.Toast(fmt.Sprintf(self.c.Tr.DesktimersSetupRepoUpdated, name))
			self.c.Refresh(types.RefreshOptions{Mode: types.ASYNC, Scope: []types.RefreshableView{types.REMOTES}})
			return nil
		},
	})
	return nil
}
