package desktimers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
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

// sectionTrailer finds a "Section:" line that names something, as the API
// reads one: anywhere in the message, any case.
var sectionTrailer = regexp.MustCompile(`(?im)^section:[ \t]*\S`)

// sectionReadLimit is how much of a message the API reads for the trailer
// (4000 characters there), less room for the summary in front of it.
const sectionReadLimit = 3800

// WithSectionTrailer ends a commit message with "Section: <name>", the line
// the API counts a commit toward a section by. A message that already names
// a section is left as it is: the person wrote it on purpose. A message too
// long for the API to read to its end gets the line at the top instead.
func WithSectionTrailer(message string, name string) string {
	if name == "" || sectionTrailer.MatchString(message) {
		return message
	}
	line := "Section: " + name
	trimmed := strings.TrimRight(message, " \t\n")
	switch {
	case trimmed == "":
		return line
	case len(trimmed) > sectionReadLimit:
		return line + "\n\n" + message
	}
	return trimmed + "\n\n" + line
}

// AddSectionTrailerToFile ends a commit message file, as the editor will
// open it, with the section's trailer. A file with nothing typed keeps its
// first line free for the summary.
func AddSectionTrailerToFile(path string, section string) error {
	if section == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	message := WithSectionTrailer(string(content), section) + "\n"
	if strings.TrimSpace(string(content)) == "" {
		message = "\n\n" + message
	}
	return os.WriteFile(path, []byte(message), 0o600)
}

// slugOf is the API's slug of a section name: "API endpoints (v2)" →
// "api-endpoints-v2".
func slugOf(name string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(name) {
		if unicode.Is(unicode.Mn, r) {
			continue // the accent split off a letter
		}
		b.WriteRune(r)
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(b.String()), "-"), "-")
	if len(slug) > 60 {
		slug = strings.TrimRight(slug[:60], "-")
	}
	return slug
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// SectionOfBranch is the ID of the section a branch is for, matched as the
// API matches it: the task's code, a separator, then a section's slug ending
// there or at another separator, the longest slug winning. "" when none.
func SectionOfBranch(branch string, taskCode string, sections []Section) string {
	m := regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])` + regexp.QuoteMeta(taskCode) + `[-/_]+(.*)$`).FindStringSubmatch(branch)
	if m == nil {
		return ""
	}
	rest := strings.ToLower(m[1])
	best, bestLen := "", 0
	for _, section := range sections {
		slug := slugOf(section.Name)
		if slug == "" {
			continue
		}
		if rest == slug || (strings.HasPrefix(rest, slug) && strings.ContainsRune("-/_", rune(rest[len(slug)]))) {
			if len(slug) > bestLen {
				best, bestLen = section.ID, len(slug)
			}
		}
	}
	return best
}
