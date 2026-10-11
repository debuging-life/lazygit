package desktimers

import (
	"testing"
)

func TestSlugOwner(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{"loudowls/deskgit", "loudowls"},
		{"debuging-life/lazygit", "debuging-life"},
		{"", ""},        // no origin remote → RepoSlug returns ""
		{"noslash", ""}, // unparseable
	}
	for _, tt := range tests {
		if got := SlugOwner(tt.slug); got != tt.want {
			t.Errorf("SlugOwner(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}

func TestOwnerMatchesOrgs(t *testing.T) {
	orgs := []string{"debuging-life", "loudowls"}

	tests := []struct {
		name  string
		owner string
		want  bool
	}{
		{"exact match", "loudowls", true},
		{"case-insensitive match", "LoudOwls", true},
		{"non-org repo", "jesseduffield", false},
		{"empty owner (no remote) never matches", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OwnerMatchesOrgs(tt.owner, orgs); got != tt.want {
				t.Errorf("OwnerMatchesOrgs(%q) = %v, want %v", tt.owner, got, tt.want)
			}
		})
	}

	if OwnerMatchesOrgs("loudowls", nil) {
		t.Error("empty org list must never match")
	}
}

func TestRemoteURLToSlug(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"git@github.com:loudowls/deskgit.git", "loudowls/deskgit"},
		{"git@github-alias:debuging-life/lazygit.git", "debuging-life/lazygit"},
		{"https://github.com/loudowls/deskgit.git", "loudowls/deskgit"},
		{"https://github.com/loudowls/deskgit", "loudowls/deskgit"},
		{"ssh://git@github.com/loudowls/deskgit.git", "loudowls/deskgit"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, tt := range tests {
		if got := remoteURLToSlug(tt.url); got != tt.want {
			t.Errorf("remoteURLToSlug(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
