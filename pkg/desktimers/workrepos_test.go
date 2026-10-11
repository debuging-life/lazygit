package desktimers

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestGetWorkRepos(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.workspaces = []map[string]any{{"id": "ws1", "name": "Acme", "slug": "acme"}}
	fake.projects["ws1"] = []map[string]any{{"code": "CLK", "name": "Clock"}, {"code": "MOB", "name": "Mobile"}, {"code": "OPS", "name": "Ops"}}
	fake.repositories["ws1/CLK"] = []string{"acme/clock", "acme/shared"}
	fake.repositories["ws1/MOB"] = []string{"Acme/Shared", "acme/mobile"}
	// One project's list cannot be read: it costs that project, not the rest.
	fake.refuse["/api/workspaces/ws1/projects/OPS/repositories"] = "no"

	repos, err := fake.client().GetWorkRepos()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, repo := range repos {
		names = append(names, strings.ToLower(repo.FullName))
		// A linked repository vouches for itself, not for its owner's others.
		if repo.Owner != "" {
			t.Errorf("%s must not carry an owner", repo.FullName)
		}
	}
	sort.Strings(names)
	if got, want := strings.Join(names, ","), "acme/clock,acme/mobile,acme/shared"; got != want {
		t.Errorf("repos = %s, want %s", got, want)
	}
	if RepoIsWork("acme/other", repos, nil) {
		t.Error("another repository of the same owner is not a work repo")
	}
	if !RepoIsWork("ACME/clock", repos, nil) {
		t.Error("a linked repository is a work repo, in any letter case")
	}
}

func TestRepoIsWork(t *testing.T) {
	serverRepos := []WorkRepo{
		{FullName: "acme/product", Owner: "acme"},
		{FullName: "solo-dev/client-work", Owner: ""}, // repo-level entry, no owner-wide match
	}
	hookOrgs := []string{"loudowls"}

	tests := []struct {
		name string
		slug string
		want bool
	}{
		{"full-name match", "acme/product", true},
		{"full-name match is case-insensitive", "Acme/Product", true},
		{"owner-wide server match", "acme/other-repo", true},
		{"repo-level entry matches only that repo", "solo-dev/client-work", true},
		{"repo-level entry does not cover siblings", "solo-dev/personal", false},
		{"hookOrgs manual supplement", "loudowls/anything", true},
		{"unknown repo", "randomer/repo", false},
		{"no origin remote", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepoIsWork(tt.slug, serverRepos, hookOrgs); got != tt.want {
				t.Errorf("RepoIsWork(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}

	// No server cache at all → only hookOrgs applies (pre-deploy behavior).
	if !RepoIsWork("loudowls/x", nil, hookOrgs) {
		t.Error("hookOrgs must keep working without a server cache")
	}
	if RepoIsWork("acme/product", nil, hookOrgs) {
		t.Error("nothing should match without cache or org hit")
	}
}

func TestCachedWorkRepos(t *testing.T) {
	now := time.Now()

	if repos, fresh := cachedWorkRepos(nil, now); repos != nil || fresh {
		t.Error("missing cache → nil, stale")
	}

	fresh := &workRepoCache{Repos: []WorkRepo{{FullName: "a/b"}}, FetchedAt: now.Add(-time.Hour)}
	if repos, ok := cachedWorkRepos(fresh, now); len(repos) != 1 || !ok {
		t.Error("hour-old cache should be fresh and usable")
	}

	stale := &workRepoCache{Repos: []WorkRepo{{FullName: "a/b"}}, FetchedAt: now.Add(-25 * time.Hour)}
	repos, ok := cachedWorkRepos(stale, now)
	if ok {
		t.Error("25h-old cache must report stale")
	}
	if len(repos) != 1 {
		t.Error("stale cache data must still be returned for matching")
	}
}
