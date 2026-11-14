package helpers

import (
	"fmt"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

// SectionChoice is what a new-branch flow does with a section picked for it.
type SectionChoice struct {
	// New makes the section's branch, named exactly as DeskTick suggests.
	New func(branch string) error
	// Existing goes to the branch the section already has.
	Existing func(branch string) error
}

// WithSectionFor asks, for a task with sections, whether the branch is for
// the whole task or one section. A task without sections, or sections that
// cannot be read (an older API, a dropped connection), goes on as the whole
// task: a branch is never held up by them.
func (self *DesktimersHelper) WithSectionFor(taskCode string, whole func() error, choice SectionChoice) error {
	if taskCode == "" {
		return whole()
	}
	var sections []desktimers.Section
	err := self.c.WithWaitingStatusSync(self.c.Tr.DesktimersLoadingSections, func() error {
		client, err := desktimers.NewClientFromToken()
		if err != nil {
			return err
		}
		sections, err = client.GetSections(taskCode)
		return err
	})
	if err != nil {
		self.c.Log.Warnf("deskgit: reading the sections of %s: %v", taskCode, err)
		return whole()
	}
	if len(sections) == 0 {
		return whole()
	}

	tr := self.c.Tr
	accent, dim := self.hudAccent(), self.hudDim()
	items := []*types.MenuItem{{
		LabelColumns: []string{accent.Sprint(tr.DesktimersSectionWholeTask), "", ""},
		OnPress:      whole,
		Keys:         menuKey('w'),
	}}
	for _, row := range desktimers.SectionRows(sections) {
		section, branch := row.Section, row.Branch
		tag := tr.DesktimersSectionNew
		var press func() error
		var disabled *types.DisabledReason
		switch {
		case row.Existing:
			tag = tr.DesktimersSectionSwitch
			press = func() error { return choice.Existing(branch) }
		case branch == "":
			disabled = &types.DisabledReason{Text: tr.DesktimersSectionNoBranch}
		default:
			press = func() error { return choice.New(branch) }
		}
		switch {
		case section.Done:
			tag += " · " + tr.DesktimersSectionDone
		case section.Blocked != nil:
			tag += " · " + tr.DesktimersSectionBlocked
		case section.Running:
			tag += " · " + tr.DesktimersSectionRunning
		}
		name := utils.TruncateWithEllipsis(section.Name, 50)
		if section.Done {
			name = dim.Sprint(name)
		}
		items = append(items, &types.MenuItem{
			LabelColumns:   []string{name, branch, dim.Sprint(tag)},
			OnPress:        press,
			DisabledReason: disabled,
		})
	}

	return self.c.Menu(types.CreateMenuOptions{
		Title:    utils.ResolvePlaceholderString(tr.DesktimersSectionsTitle, map[string]string{"code": taskCode}),
		Items:    items,
		MinWidth: 100,
	})
}

// sectionChoice is what the branches panel does with a section: make its
// branch from `from`, or go to the one it has.
func (self *RefsHelper) sectionChoice(from string) *SectionChoice {
	return &SectionChoice{
		New: func(branch string) error { return self.newSectionBranch(from, branch) },
		Existing: func(branch string) error {
			self.c.LogAction(self.c.Tr.Actions.CheckoutBranch)
			return self.CheckoutRef(branch, types.CheckoutRefOptions{})
		},
	}
}

// newSectionBranch makes the section's branch, named exactly as DeskTick
// gave it (no prompt to change it: the name is how the API finds the
// section). It does not track `from`: a branch with a name of its own must
// not push to its base's upstream.
func (self *RefsHelper) newSectionBranch(from string, branch string) error {
	self.c.LogAction(self.c.Tr.Actions.CreateBranch)
	create := func() error { return self.c.Git().Branch.NewWithoutTracking(branch, from) }
	refresh := func() {
		if self.c.Context().Current() != self.c.Contexts().Branches {
			self.c.Context().Push(self.c.Contexts().Branches, types.OnFocusOpts{})
		}
		self.SelectFirstBranchAndFirstCommit()
		self.c.Refresh(types.RefreshOptions{
			Mode:                     types.BLOCK_UI,
			KeepBranchSelectionIndex: true,
			CommitSelection:          types.KeepCommitSelectionIndex,
		})
	}

	err := create()
	if err == nil {
		refresh()
		return nil
	}
	if !IsSwitchBranchUncommittedChangesError(err) {
		return err
	}
	// The same offer as any new branch: stash, switch, put the changes back.
	self.c.Confirm(types.ConfirmOpts{
		Title:  self.c.Tr.AutoStashTitle,
		Prompt: self.c.Tr.AutoStashPrompt,
		HandleConfirm: func() error {
			if err := self.c.Git().Stash.Push(fmt.Sprintf(self.c.Tr.AutoStashForNewBranch, branch)); err != nil {
				return err
			}
			if err := create(); err != nil {
				return err
			}
			err := self.c.Git().Stash.Pop(0)
			refresh()
			return err
		},
	})
	return nil
}
