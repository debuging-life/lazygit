package helpers

import (
	"errors"

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
	tr := self.c.Tr
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
		// Said, not hidden: a whole-task branch made instead loses the
		// section, and the person may rather retry.
		self.c.Log.Warnf("deskgit: reading the sections of %s: %v", taskCode, err)
		self.c.ErrorToast(utils.ResolvePlaceholderString(tr.DesktimersSectionsUnread, map[string]string{"error": err.Error()}))
		return whole()
	}
	if len(sections) == 0 {
		return whole()
	}

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
		case section.Done:
			// Its work is finished: switching to a branch it had is fine,
			// starting one is not.
			disabled = &types.DisabledReason{Text: tr.DesktimersSectionDoneNoBranch}
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
		New: func(branch string) error {
			// Made here before, and nothing of it has reached DeskTick yet.
			if self.hasLocalBranch(branch) {
				return self.switchToSectionBranch(branch)
			}
			// Named exactly as DeskTick gave it, no prompt to change it: the
			// name is how the API finds the section. Not tracking `from`: a
			// branch with a name of its own must not push to its base's
			// upstream.
			self.c.LogAction(self.c.Tr.Actions.CreateBranch)
			return self.createBranch(branch, from, self.c.Git().Branch.NewWithoutTracking)
		},
		Existing: self.switchToSectionBranch,
	}
}

func (self *RefsHelper) hasLocalBranch(name string) bool {
	for _, branch := range self.c.Model().Branches {
		if branch.Name == name {
			return true
		}
	}
	return false
}

// switchToSectionBranch goes to a section's branch; one DeskTick knows of
// that this clone does not (a teammate's, not fetched) is said in words.
func (self *RefsHelper) switchToSectionBranch(branch string) error {
	self.c.LogAction(self.c.Tr.Actions.CheckoutBranch)
	return self.CheckoutRef(branch, types.CheckoutRefOptions{
		OnRefNotFound: func(ref string) error {
			return errors.New(utils.ResolvePlaceholderString(self.c.Tr.DesktimersSectionBranchNotHere, map[string]string{"branch": ref}))
		},
	})
}
