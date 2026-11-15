package desktimers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSectionsAsksTheTasksProject(t *testing.T) {
	f := newFakeDeskTick(t)
	f.oneProject()
	branch := "CLK-2/api-endpoints"
	f.sections = map[string][]map[string]any{"ws1/CLK/CLK-2": {
		{
			"id": "s1", "name": "API endpoints", "done": false, "running": true, "blocked": nil,
			"code": map[string]any{"branch": branch, "commits": 3}, "suggestedBranch": branch,
		},
		{
			"id": "s2", "name": "Web page", "done": false, "running": false,
			"blocked": map[string]any{"reason": "waiting on design", "at": "2026-11-14T10:00:00Z", "by": "u1"},
			"code":    map[string]any{"branch": nil, "commits": 0}, "suggestedBranch": "CLK-2/web-page",
		},
	}}

	sections, err := f.client().GetSections("clk-2")
	assert.NoError(t, err)
	assert.Len(t, sections, 2)
	assert.Equal(t, branch, sections[0].ExistingBranch())
	assert.Equal(t, 3, sections[0].Code.Commits)
	assert.True(t, sections[0].Running)
	assert.Equal(t, "", sections[1].ExistingBranch())
	assert.Equal(t, "CLK-2/web-page", sections[1].SuggestedBranch)
	assert.Equal(t, "waiting on design", sections[1].Blocked.Reason)
	assert.Equal(t, []string{"GET /api/workspaces/ws1/projects/CLK/tasks/CLK-2/sections"}, f.asked("GET /api/workspaces/ws1/projects/CLK/tasks"))
}

func TestATaskWithoutSectionsIsAnEmptyList(t *testing.T) {
	f := newFakeDeskTick(t)
	f.oneProject()

	sections, err := f.client().GetSections("CLK-1")
	assert.NoError(t, err)
	assert.Empty(t, sections)
}

func TestSectionsRefusedInTheAPIsWords(t *testing.T) {
	f := newFakeDeskTick(t)
	f.oneProject()
	f.refuse["/api/workspaces/ws1/projects/CLK/tasks/CLK-1/sections"] = "You cannot see this task."

	_, err := f.client().GetSections("CLK-1")
	assert.ErrorContains(t, err, "You cannot see this task.")
}

func TestDoneSectionsGoLastInTheirOrder(t *testing.T) {
	sections := []Section{{Name: "a", Done: true}, {Name: "b"}, {Name: "c", Done: true}, {Name: "d"}}
	names := []string{}
	for _, s := range SectionsForPicker(sections) {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{"b", "d", "a", "c"}, names)
	assert.Equal(t, "a", sections[0].Name, "the caller's list is left as it was")
}

func TestASectionWithABranchIsSwitchedTo(t *testing.T) {
	had := "CLK-2/api"
	sections := []Section{
		{Name: "done one", Done: true, SuggestedBranch: "CLK-2/done-one"},
		{Name: "api", SuggestedBranch: "CLK-2/api-endpoints"},
		{Name: "web", SuggestedBranch: "CLK-2/web"},
		{Name: "nameless"},
	}
	sections[1].Code.Branch = &had

	rows := SectionRows(sections)
	assert.Equal(t, []SectionRow{
		{Section: sections[1], Branch: "CLK-2/api", Existing: true},
		{Section: sections[2], Branch: "CLK-2/web"},
		{Section: sections[3], Branch: ""},
		{Section: sections[0], Branch: "CLK-2/done-one"},
	}, rows)
}

func TestWithSectionTrailer(t *testing.T) {
	cases := []struct{ name, message, section, want string }{
		{"no section", "body", "", "body"},
		{"empty description", "", "API endpoints", "Section: API endpoints"},
		{"after the body", "Wire the route\n", "API endpoints", "Wire the route\n\nSection: API endpoints"},
		{"already named", "body\n\nsection: Docs", "API endpoints", "body\n\nsection: Docs"},
		{"an empty Section line names nothing", "body\nSection:", "API endpoints", "body\nSection:\n\nSection: API endpoints"},
		{"the word in a sentence is not a trailer", "this section: hard", "API endpoints", "this section: hard\n\nSection: API endpoints"},
	}
	for _, c := range cases {
		if got := WithSectionTrailer(c.message, c.section); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWithSectionTrailerOnALongMessageGoesFirst(t *testing.T) {
	long := strings.Repeat("log line\n", 500)
	got := WithSectionTrailer(long, "API endpoints")
	assert.True(t, strings.HasPrefix(got, "Section: API endpoints\n\n"), "the API reads only the first 4000 characters")
	assert.True(t, strings.HasSuffix(got, long))
}

func TestAddSectionTrailerToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "msg")
	assert.NoError(t, os.WriteFile(path, nil, 0o600))
	assert.NoError(t, AddSectionTrailerToFile(path, "API endpoints"))
	got, _ := os.ReadFile(path)
	assert.Equal(t, "\n\nSection: API endpoints\n", string(got), "the first line stays free for the summary")

	assert.NoError(t, os.WriteFile(path, []byte("Wire it\n"), 0o600))
	assert.NoError(t, AddSectionTrailerToFile(path, "API endpoints"))
	got, _ = os.ReadFile(path)
	assert.Equal(t, "Wire it\n\nSection: API endpoints\n", string(got))
}

func TestSectionOfBranchMatchesAsTheAPIDoes(t *testing.T) {
	sections := []Section{{ID: "a", Name: "API"}, {ID: "t", Name: "API tests"}, {ID: "c", Name: "Café menu"}}
	cases := map[string]string{
		"LOUD-4/api":          "a",
		"LOUD-4/api-tests":    "t", // the longest slug wins
		"loud-4/API-tests-v2": "t", // any case, a separator after the slug
		"feature/loud-4-api":  "a",
		"LOUD-4/cafe-menu":    "c", // the accent dropped, as the API's slug does
		"LOUD-4/apis":         "",  // the slug must end at a separator
		"LOUD-4":              "",
		"XLOUD-4/api":         "", // the code must stand alone
		"LOUD-44/api":         "",
	}
	for branch, want := range cases {
		assert.Equal(t, want, SectionOfBranch(branch, "LOUD-4", sections), branch)
	}
}
