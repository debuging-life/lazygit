package desktimers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetWorkRepos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/git-client/repos" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer dtg_abc" {
			t.Errorf("missing bearer, got %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"success":true,"data":[{"fullName":"debuging-life/loudowls","owner":"debuging-life"}]}`))
	}))
	defer server.Close()

	repos, err := NewClient(server.URL, "dtg_abc").GetWorkRepos()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].FullName != "debuging-life/loudowls" || repos[0].Owner != "debuging-life" {
		t.Errorf("unexpected repos: %+v", repos)
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
