package desktimers

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// A task can be divided into sections in its description, each with a
// branch of its own. The API decides which branch belongs to a section (the
// task's code followed by the section's slug, the longest slug winning), so
// deskgit never builds the name: it takes suggestedBranch as it is.

// Section is one section of a task, as GET …/tasks/:code/sections answers.
type Section struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Done    bool   `json:"done"`
	Running bool   `json:"running"`
	// Blocked is set while somebody has said the section is held up.
	Blocked *struct {
		Reason string `json:"reason"`
	} `json:"blocked"`
	Code struct {
		// Branch is a branch already counted for the section, if any.
		Branch  *string `json:"branch"`
		Commits int     `json:"commits"`
	} `json:"code"`
	SuggestedBranch string `json:"suggestedBranch"`
}

// ExistingBranch is the branch the section already has, or "".
func (s Section) ExistingBranch() string {
	if s.Code.Branch == nil {
		return ""
	}
	return *s.Code.Branch
}

// GetSections returns the task's sections in the description's order; a
// task with none is an empty list.
func (c *Client) GetSections(taskCode string) ([]Section, error) {
	place, err := c.placeOf(taskCode)
	if err != nil {
		return nil, err
	}
	var got struct {
		Sections []Section `json:"sections"`
	}
	path := place.path() + "/tasks/" + url.PathEscape(strings.ToUpper(taskCode)) + "/sections"
	if err := c.call(http.MethodGet, path, nil, &got); err != nil {
		return nil, fmt.Errorf("reading the task's sections: %w", err)
	}
	return got.Sections, nil
}

// SectionsForPicker keeps the description's order but puts done sections
// last, as the web does.
func SectionsForPicker(sections []Section) []Section {
	out := append([]Section(nil), sections...)
	sort.SliceStable(out, func(i, j int) bool { return !out[i].Done && out[j].Done })
	return out
}

// SectionRow is a section as the branch picker offers it: the branch it
// already has, to switch to, or else the one DeskTick suggests, to make.
// Branch is "" when DeskTick gave neither.
type SectionRow struct {
	Section  Section
	Branch   string
	Existing bool
}

// SectionRows orders the sections for the picker (done last) and says, for
// each, which branch picking it means.
func SectionRows(sections []Section) []SectionRow {
	rows := []SectionRow{}
	for _, section := range SectionsForPicker(sections) {
		row := SectionRow{Section: section, Branch: section.SuggestedBranch}
		if existing := section.ExistingBranch(); existing != "" {
			row.Branch, row.Existing = existing, true
		}
		rows = append(rows, row)
	}
	return rows
}

// sectionTrailer finds a "Section:" line, as the API reads one: anywhere in
// the message, any case.
var sectionTrailer = regexp.MustCompile(`(?im)^section:`)

// WithSectionTrailer ends a commit message with "Section: <name>", the line
// the API counts a commit toward a section by. A message that already names
// a section is left as it is: the person wrote it on purpose.
func WithSectionTrailer(message string, name string) string {
	if name == "" || sectionTrailer.MatchString(message) {
		return message
	}
	trimmed := strings.TrimRight(message, " \t\n")
	if trimmed == "" {
		return "Section: " + name
	}
	return trimmed + "\n\nSection: " + name
}
