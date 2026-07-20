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

// DesktimersHelper owns the dtgit-specific UI: the task picker, the
// status-line segment, branch-name prefixes, and the hook install prompt.
type DesktimersHelper struct {
	c *HelperCommon

	// cached selected-task state so the status line doesn't hit the disk on
	// every layout pass. nil = not loaded yet or no selection.
	mutex        sync.Mutex
	cachedState  *desktimers.State
	stateLoaded  bool
	hooksChecked bool
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

	menuItems := lo.Map(tasks, func(task desktimers.Task, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: desktimers.MenuColumns(task),
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

	return self.c.Menu(types.CreateMenuOptions{
		Title: self.c.Tr.DesktimersTaskMenuTitle,
		Items: menuItems,
	})
}

func (self *DesktimersHelper) selectTask(task desktimers.Task) error {
	state := desktimers.NewState(task)
	if err := desktimers.SaveState(".", state); err != nil {
		return err
	}
	self.setSelectedTask(state)
	return nil
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
