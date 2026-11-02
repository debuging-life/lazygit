package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

// ── the pull request form ───────────────────────────────────────────────────
// One menu with every field of a pull request and its value; a row is
// edited by pressing it, and the form comes back with the new value. What
// can be picked (people, teams, labels, projects) is asked of GitHub once,
// when the form opens. See ../docs/features/deskgit/README.md.

// PullRequestFormOpts is what the branches panel knows when the form opens.
type PullRequestFormOpts struct {
	Repo desktimers.GitHubRepo
	// Branch is the local branch; Head the branch on GitHub.
	Branch string
	Head   string
	// Base is where the pull request goes; "" is the repository's default.
	Base string
	// EditFile opens a file in the person's editor and returns when it is
	// closed.
	EditFile func(path string) error
	// BaseSuggestions suggests branches for the base.
	BaseSuggestions func(string) []*types.Suggestion
	// OnCreated shows what can be done with the pull request made, into
	// base; note is what GitHub refused of the rest, or "".
	OnCreated func(number int, url string, base string, note string) error
	// Reopen starts again from the branches panel, asked as another gh
	// account.
	Reopen func() error
}

// pullRequestForm is the form as filled in so far.
type pullRequestForm struct {
	opts    PullRequestFormOpts
	choices *desktimers.PullRequestChoices
	draft   desktimers.PullRequestDraft
	// accounts are the gh accounts the form could be asked as.
	accounts desktimers.GitHubAccounts
	// fromTemplate says the description is still the repository's template.
	fromTemplate bool
}

// The form's rows, in order; a row's index is where the form comes back to.
const (
	prRowTitle = iota
	prRowDescription
	prRowBase
	prRowReviewers
	prRowAssignees
	prRowLabels
	prRowProjects
	prRowAccount
	prRowCreate
	prRowCreateDraft
)

// teamPrefix marks a team among people in the reviewer picker.
const teamPrefix = "team:"

// OpenPullRequestForm asks GitHub what can go on a pull request and works
// out a title and a description, then shows the form.
func (self *DesktimersHelper) OpenPullRequestForm(opts PullRequestFormOpts) error {
	picked := self.SelectedTask()
	return self.c.WithWaitingStatus(self.c.Tr.PRFormLoading, func(gocui.Task) error {
		// The task's link is DeskTick's to give, and does not wait on GitHub.
		taskURL := make(chan string, 1)
		go func() {
			code := desktimers.ExtractCode(opts.Branch)
			if code == "" && picked != nil {
				code = picked.Code
			}
			if code == "" {
				taskURL <- ""
				return
			}
			taskURL <- desktimers.TaskURLForCode(".", code)
		}()
		accounts := desktimers.ListGitHubAccounts(opts.Repo.Host)
		choices, err := opts.Repo.LoadPullRequestChoices()
		if errors.Is(err, desktimers.ErrRepoNotFound) {
			// Usually the account, not the repository: with another one to
			// try, offer it.
			if len(accounts.Logins) > 1 && opts.Reopen != nil {
				self.c.OnUIThread(func() error {
					return self.ChooseGitHubAccount(opts.Repo.Host, err.Error()+". "+self.c.Tr.GitHubAccountPickAnother, opts.Reopen)
				})
				return nil
			}
			return errors.New(err.Error() + ". " + self.c.Tr.GitHubAccountSignInHint)
		}
		if err != nil {
			return err
		}
		base := opts.Base
		if base == "" {
			base = choices.DefaultBranch
		}
		text := desktimers.DefaultPullRequestText(".", opts.Branch, "origin/"+base, picked, <-taskURL)

		form := &pullRequestForm{
			opts:    opts,
			choices: choices,
			draft: desktimers.PullRequestDraft{
				Title: text.Title,
				Body:  text.Body,
				Head:  opts.Head,
				Base:  base,
			},
			fromTemplate: text.FromTemplate,
			accounts:     accounts,
		}
		self.c.OnUIThread(func() error { return self.showPullRequestForm(form, prRowTitle) })
		return nil
	})
}

func (self *DesktimersHelper) showPullRequestForm(form *pullRequestForm, selected int) error {
	tr := self.c.Tr
	details := &types.MenuSection{Title: tr.PRFormSectionDetails}
	create := &types.MenuSection{Title: tr.PRFormSectionCreate}
	back := func(row int) func() error {
		return func() error { return self.showPullRequestForm(form, row) }
	}
	row := func(key rune, label string, value string, onPress func() error) *types.MenuItem {
		return &types.MenuItem{LabelColumns: []string{label, value}, OnPress: onPress, Section: details, Keys: menuKey(key)}
	}

	projects := row('p', tr.PRFieldProjects, self.listOrNone(projectTitles(form)), func() error {
		return self.pickForPullRequest(form, prRowProjects)
	})
	if errors.Is(form.choices.ProjectsErr, desktimers.ErrProjectsNeedScope) {
		projects.LabelColumns[1] = tr.PRFormProjectsNeedScopeShort
		projects.DisabledReason = &types.DisabledReason{Text: tr.PRFormProjectsNeedScope, ShowErrorInPanel: true}
	} else if form.choices.ProjectsErr != nil {
		projects.LabelColumns[1] = tr.PRFormUnavailable
		projects.DisabledReason = &types.DisabledReason{Text: form.choices.ProjectsErr.Error()}
	}

	items := []*types.MenuItem{
		row('t', tr.PRFieldTitle, form.draft.Title, func() error {
			self.c.Prompt(types.PromptOpts{
				Title:          tr.PRFieldTitle,
				InitialContent: form.draft.Title,
				HandleConfirm: func(title string) error {
					form.draft.Title = strings.TrimSpace(title)
					return self.showPullRequestForm(form, prRowTitle)
				},
				HandleClose: back(prRowTitle),
			})
			return nil
		}),
		row('e', tr.PRFieldDescription, self.describeBody(form), func() error {
			return self.editPullRequestBody(form)
		}),
		row('b', tr.PRFieldBase, form.draft.Base, func() error {
			self.c.Prompt(types.PromptOpts{
				Title:               tr.PRBaseBranch,
				InitialContent:      form.draft.Base,
				FindSuggestionsFunc: form.opts.BaseSuggestions,
				HandleConfirm: func(base string) error {
					if base = strings.TrimPrefix(strings.TrimSpace(base), "origin/"); base != "" {
						form.draft.Base = base
					}
					return self.showPullRequestForm(form, prRowBase)
				},
				HandleClose: back(prRowBase),
			})
			return nil
		}),
		row('r', tr.PRFieldReviewers, self.listOrNone(reviewerNames(form)), func() error {
			return self.pickForPullRequest(form, prRowReviewers)
		}),
		row('a', tr.PRFieldAssignees, self.listOrNone(form.draft.Assignees), func() error {
			return self.pickForPullRequest(form, prRowAssignees)
		}),
		row('l', tr.PRFieldLabels, self.listOrNone(form.draft.Labels), func() error {
			return self.pickForPullRequest(form, prRowLabels)
		}),
		projects,
		self.accountRow(form, details),
		{
			LabelColumns: []string{tr.PRFormCreate, ""},
			OnPress:      func() error { return self.createPullRequest(form, false) },
			Section:      create,
			Keys:         menuKey('c'),
		},
		{
			LabelColumns: []string{tr.PRFormCreateDraft, ""},
			OnPress:      func() error { return self.createPullRequest(form, true) },
			Section:      create,
			Keys:         menuKey('d'),
		},
	}

	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(tr.PRFormTitle, map[string]string{
			"head": form.draft.Head,
			"base": form.draft.Base,
		}),
		Items:              items,
		InitialSelectedIdx: selected,
		MinWidth:           100,
		// Esc out of a form somebody has filled in asks first: the
		// description may be minutes of writing.
		OnCancel: func() error {
			self.c.Confirm(types.ConfirmOpts{
				Title:         tr.PRFormDiscardTitle,
				Prompt:        tr.PRFormDiscardPrompt,
				HandleConfirm: func() error { return nil },
				HandleClose:   back(selected),
			})
			return nil
		},
	})
}

func (self *DesktimersHelper) listOrNone(values []string) string {
	if len(values) == 0 {
		return self.c.Tr.PRFormNone
	}
	return strings.Join(values, ", ")
}

func (self *DesktimersHelper) describeBody(form *pullRequestForm) string {
	body := strings.TrimSpace(form.draft.Body)
	if body == "" {
		return self.c.Tr.PRFormNone
	}
	first, _, _ := strings.Cut(body, "\n")
	described := utils.ResolvePlaceholderString(self.c.Tr.PRFormDescriptionLines, map[string]string{
		"first": utils.TruncateWithEllipsis(first, 40),
		"lines": fmt.Sprintf("%d", strings.Count(body, "\n")+1),
	})
	if form.fromTemplate {
		described += " " + self.c.Tr.PRFormFromTemplate
	}
	return described
}

// editPullRequestBody opens the description in the person's editor, as
// git does a commit message.
func (self *DesktimersHelper) editPullRequestBody(form *pullRequestForm) error {
	// Whatever goes wrong, the form comes back as it was.
	if err := self.editBodyInEditor(form); err != nil {
		self.c.ErrorToast(err.Error())
	}
	return self.showPullRequestForm(form, prRowDescription)
}

func (self *DesktimersHelper) editBodyInEditor(form *pullRequestForm) error {
	dir, err := os.MkdirTemp("", "deskgit-pr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "PULL_REQUEST_DESCRIPTION.md")
	if err := os.WriteFile(path, []byte(form.draft.Body+"\n"), 0o600); err != nil {
		return err
	}
	if err := form.opts.EditFile(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if body := strings.TrimRight(string(data), "\n"); body != form.draft.Body {
		form.draft.Body = body
		form.fromTemplate = false
	}
	return nil
}

// pickOption is one row of a picker: what it shows, what it sends, and
// whether it is ticked.
type pickOption struct {
	label  string
	detail string
	value  string
}

func (self *DesktimersHelper) pickOptions(form *pullRequestForm, row int) (options []pickOption, ticked []string) {
	choices := form.choices
	switch row {
	case prRowReviewers:
		for _, user := range choices.ReviewerCandidates() {
			options = append(options, pickOption{label: user.Login, value: user.Login})
		}
		for _, team := range choices.Teams {
			options = append(options, pickOption{
				label: "@" + form.opts.Repo.Owner + "/" + team.Slug, detail: team.Name, value: teamPrefix + team.Slug,
			})
		}
		ticked = append(slices.Clone(form.draft.Reviewers), prefixed(teamPrefix, form.draft.TeamReviewers)...)
	case prRowAssignees:
		for _, user := range choices.Users {
			option := pickOption{label: user.Login, value: user.Login}
			if strings.EqualFold(user.Login, choices.Viewer) {
				option.detail = self.c.Tr.PRFormYou
				options = append([]pickOption{option}, options...)
				continue
			}
			options = append(options, option)
		}
		ticked = form.draft.Assignees
	case prRowLabels:
		for _, label := range choices.Labels {
			options = append(options, pickOption{label: label.Name, detail: label.Description, value: label.Name})
		}
		ticked = form.draft.Labels
	case prRowProjects:
		for _, project := range choices.Projects {
			options = append(options, pickOption{label: project.Title, detail: fmt.Sprintf("#%d", project.Number), value: project.ID})
		}
		ticked = form.draft.ProjectIDs
	}
	return options, ticked
}

func (self *DesktimersHelper) setPicked(form *pullRequestForm, row int, values []string) {
	switch row {
	case prRowReviewers:
		form.draft.Reviewers, form.draft.TeamReviewers = nil, nil
		for _, value := range values {
			if slug, ok := strings.CutPrefix(value, teamPrefix); ok {
				form.draft.TeamReviewers = append(form.draft.TeamReviewers, slug)
			} else {
				form.draft.Reviewers = append(form.draft.Reviewers, value)
			}
		}
	case prRowAssignees:
		form.draft.Assignees = values
	case prRowLabels:
		form.draft.Labels = values
	case prRowProjects:
		form.draft.ProjectIDs = values
	}
}

// pickForPullRequest is a list to tick: Enter ticks or unticks a row and
// leaves the list open; Esc goes back to the form with what is ticked.
func (self *DesktimersHelper) pickForPullRequest(form *pullRequestForm, row int) error {
	return self.showPicker(form, row, 0)
}

func (self *DesktimersHelper) showPicker(form *pullRequestForm, row int, selected int) error {
	options, ticked := self.pickOptions(form, row)
	if len(options) == 0 {
		self.c.ErrorToast(self.c.Tr.PRFormNoChoices)
		return self.showPullRequestForm(form, row)
	}
	items := make([]*types.MenuItem, len(options), len(options)+1)
	for i, option := range options {
		items[i] = &types.MenuItem{
			LabelColumns: []string{option.label, option.detail},
			Widget:       types.MakeMenuCheckBox(slices.Contains(ticked, option.value)),
			OnPress: func() error {
				_, ticked := self.pickOptions(form, row)
				if slices.Contains(ticked, option.value) {
					ticked = slices.DeleteFunc(slices.Clone(ticked), func(v string) bool { return v == option.value })
				} else {
					ticked = append(slices.Clone(ticked), option.value)
				}
				self.setPicked(form, row, ticked)
				return self.showPicker(form, row, i)
			},
		}
	}
	// Ticking keeps the list open, so its way out is "done", not "cancel":
	// what is ticked stays ticked either way.
	items = append(items, &types.MenuItem{
		LabelColumns: []string{self.c.Tr.PRFormDone, ""},
		OnPress:      func() error { return self.showPullRequestForm(form, row) },
	})
	return self.c.Menu(types.CreateMenuOptions{
		Title: utils.ResolvePlaceholderString(self.c.Tr.PRFormPickerTitle, map[string]string{
			"field": self.fieldName(row),
		}),
		Items:              items,
		InitialSelectedIdx: selected,
		HideCancel:         true,
		OnCancel:           func() error { return self.showPullRequestForm(form, row) },
	})
}

func (self *DesktimersHelper) fieldName(row int) string {
	tr := self.c.Tr
	switch row {
	case prRowReviewers:
		return tr.PRFieldReviewers
	case prRowAssignees:
		return tr.PRFieldAssignees
	case prRowLabels:
		return tr.PRFieldLabels
	default:
		return tr.PRFieldProjects
	}
}

func (self *DesktimersHelper) createPullRequest(form *pullRequestForm, asDraft bool) error {
	if form.draft.Title == "" {
		self.c.ErrorToast(self.c.Tr.PRFormTitleRequired)
		return self.showPullRequestForm(form, prRowTitle)
	}
	if form.draft.Base == form.draft.Head {
		self.c.ErrorToast(self.c.Tr.PRSameBranch)
		return self.showPullRequestForm(form, prRowBase)
	}
	draft := form.draft
	draft.Draft = asDraft
	self.c.LogAction(self.c.Tr.Actions.CreatePullRequestViaAPI)
	return self.c.WithWaitingStatus(self.c.Tr.PRCreatingPR, func(gocui.Task) error {
		created, err := form.opts.Repo.CreatePullRequest(draft)
		if err != nil {
			// Nothing was made: the form comes back as it was, to try again.
			self.c.OnUIThread(func() error {
				self.c.ErrorToast(err.Error())
				return self.showPullRequestForm(form, prRowCreate)
			})
			return nil
		}
		note := ""
		if len(created.Warnings) > 0 {
			note = utils.ResolvePlaceholderString(self.c.Tr.PRFormCreatedWithWarnings, map[string]string{
				"warnings": strings.Join(created.Warnings, "\n"),
			})
		}
		self.c.OnUIThread(func() error {
			self.c.Refresh(types.RefreshOptions{Scope: []types.RefreshableView{types.BRANCHES}, Mode: types.ASYNC})
			return form.opts.OnCreated(created.Number, created.URL, draft.Base, note)
		})
		return nil
	})
}

func reviewerNames(form *pullRequestForm) []string {
	names := slices.Clone(form.draft.Reviewers)
	for _, slug := range form.draft.TeamReviewers {
		names = append(names, "@"+form.opts.Repo.Owner+"/"+slug)
	}
	return names
}

func projectTitles(form *pullRequestForm) []string {
	titles := []string{}
	for _, id := range form.draft.ProjectIDs {
		for _, project := range form.choices.Projects {
			if project.ID == id {
				titles = append(titles, project.Title)
			}
		}
	}
	return titles
}

func prefixed(prefix string, values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = prefix + value
	}
	return out
}

// accountRow shows the gh account the form is asked as, and changes it
// when gh has others; changing it starts the form again, since what can be
// picked is that account's.
func (self *DesktimersHelper) accountRow(form *pullRequestForm, section *types.MenuSection) *types.MenuItem {
	item := &types.MenuItem{
		LabelColumns: []string{self.c.Tr.PRFieldAccount, form.choices.Viewer},
		Section:      section,
		Keys:         menuKey('g'),
		OnPress: func() error {
			return self.ChooseGitHubAccount(form.opts.Repo.Host, self.c.Tr.GitHubAccountChangeStartsAgain, form.opts.Reopen)
		},
	}
	if len(form.accounts.Logins) < 2 || form.opts.Reopen == nil {
		item.DisabledReason = &types.DisabledReason{Text: self.c.Tr.GitHubAccountOnlyOne}
	}
	return item
}

// ChooseGitHubAccount asks which gh account the repository is asked as,
// keeps the answer in the repository's git config, then goes on.
func (self *DesktimersHelper) ChooseGitHubAccount(host string, prompt string, then func() error) error {
	accounts := desktimers.ListGitHubAccounts(host)
	chosen := desktimers.RememberedGitHubAccount(".")
	items := make([]*types.MenuItem, 0, len(accounts.Logins))
	selected := 0
	for i, login := range accounts.Logins {
		note := ""
		switch {
		case strings.EqualFold(login, chosen):
			note = self.c.Tr.GitHubAccountChosen
			selected = i
		case login == accounts.Active:
			note = self.c.Tr.GitHubAccountActive
		}
		items = append(items, &types.MenuItem{
			LabelColumns: []string{login, note},
			OnPress: func() error {
				if err := desktimers.RememberGitHubAccount(".", login); err != nil {
					return err
				}
				return then()
			},
		})
	}
	return self.c.Menu(types.CreateMenuOptions{
		Title:              self.c.Tr.GitHubAccountTitle,
		Prompt:             prompt,
		Items:              items,
		InitialSelectedIdx: selected,
	})
}
