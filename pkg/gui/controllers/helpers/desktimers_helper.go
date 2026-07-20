package helpers

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/samber/lo"
)

// DesktimersHelper owns the deskgit-specific UI: the task picker, the
// status-line segment, branch-name prefixes, and the hook install prompt.
type DesktimersHelper struct {
	c *HelperCommon

	// cached selected-task state so the status line doesn't hit the disk on
	// every layout pass. nil = not loaded yet or no selection.
	mutex        sync.Mutex
	cachedState  *desktimers.State
	stateLoaded  bool
	hooksChecked bool
	// cached "is there a usable token" answer, same reason as above.
	authChecked   bool
	authenticated bool
}

func NewDesktimersHelper(c *HelperCommon) *DesktimersHelper {
	return &DesktimersHelper{c: c}
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
			return errors.New(self.c.Tr.DesktimersReauthNeeded)
		}
		return err
	}

	currentCode := ""
	if state := self.SelectedTask(); state != nil {
		currentCode = state.Code
	}
	ordered, selectedIdx := desktimers.OrderTasksForPicker(tasks, currentCode)

	menuItems := lo.Map(ordered, func(task desktimers.Task, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: desktimers.PickerColumns(task),
			OnPress: func() error {
				return self.selectTask(task)
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

	if self.SelectedTask() != nil {
		menuItems = append(menuItems, &types.MenuItem{
			Label: self.c.Tr.DesktimersClearTask,
			OnPress: func() error {
				if err := desktimers.ClearState("."); err != nil {
					return err
				}
				self.setSelectedTask(nil)
				return nil
			},
		})
	}

	menuItems = append(menuItems, &types.MenuItem{
		Label:   self.c.Tr.DesktimersLogout,
		OnPress: self.confirmLogout,
	})

	return self.c.Menu(types.CreateMenuOptions{
		Title:              self.c.Tr.DesktimersTaskMenuTitle,
		Items:              menuItems,
		InitialSelectedIdx: selectedIdx,
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
			return errors.New(self.c.Tr.DesktimersReauthNeeded)
		}
		return err
	}

	currentCode := ""
	if state := self.SelectedTask(); state != nil {
		currentCode = state.Code
	}
	ordered, selectedIdx := desktimers.OrderTasksForPicker(tasks, currentCode)

	menuItems := lo.Map(ordered, func(task desktimers.Task, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: desktimers.PickerColumns(task),
			OnPress: func() error {
				if err := self.selectTask(task); err != nil {
					return err
				}
				return onPick(task)
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

	return self.c.Menu(types.CreateMenuOptions{
		Title:              self.c.Tr.DesktimersPickTaskTitle,
		Items:              menuItems,
		InitialSelectedIdx: selectedIdx,
	})
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
// and, depending on desktimers.autoInstallHooks, installs or prompts.
func (self *DesktimersHelper) PromptToInstallHooksInBackground() {
	self.mutex.Lock()
	alreadyChecked := self.hooksChecked
	self.hooksChecked = true
	self.mutex.Unlock()
	if alreadyChecked {
		return
	}

	mode := self.c.UserConfig().Desktimers.AutoInstallHooks
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

	dtHookPath, ok := findDtHookBinary()
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

// findDtHookBinary looks for dt-hook next to the running executable, then on
// the PATH.
func findDtHookBinary() (string, bool) {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "dt-hook")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	if path, err := exec.LookPath("dt-hook"); err == nil {
		return path, true
	}
	return "", false
}
