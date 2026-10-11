package desktimers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkRepo is a git repo connected to one of the user's DeskTick
// workspaces; deskgit auto-installs hooks in these.
type WorkRepo struct {
	FullName string `json:"fullName"`
	Owner    string `json:"owner"`
}

// GetWorkRepos is the repositories linked to the projects the person is on
// (project settings → Repositories in the web app). Those are the ones a
// commit can be about a task in, so those are the ones that get the hooks.
// A project whose list cannot be read is skipped: one refusal does not cost
// the rest.
func (c *Client) GetWorkRepos() ([]WorkRepo, error) {
	places, err := c.places()
	if err != nil {
		return nil, err
	}
	perPlace := make([][]WorkRepo, len(places))
	_ = each(len(places), func(i int) error {
		got := struct {
			Linked []struct {
				FullName string `json:"fullName"`
			} `json:"linked"`
		}{}
		if err := c.call(http.MethodGet, places[i].path()+"/repositories", nil, &got); err != nil {
			return nil
		}
		for _, repo := range got.Linked {
			// The owner is left empty on purpose: a linked repository
			// vouches for itself, not for everything its owner has.
			perPlace[i] = append(perPlace[i], WorkRepo{FullName: repo.FullName})
		}
		return nil
	})
	repos := []WorkRepo{}
	seen := map[string]bool{}
	for _, got := range perPlace {
		for _, repo := range got {
			if key := strings.ToLower(repo.FullName); !seen[key] {
				seen[key] = true
				repos = append(repos, repo)
			}
		}
	}
	return repos, nil
}

const (
	workRepoCacheFileName = "work-repos.json"
	workRepoCacheTTL      = 24 * time.Hour
)

type workRepoCache struct {
	Repos     []WorkRepo `json:"repos"`
	FetchedAt time.Time  `json:"fetchedAt"`
}

func workRepoCachePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workRepoCacheFileName), nil
}

func loadWorkRepoCache() *workRepoCache {
	path, err := workRepoCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	cache := &workRepoCache{}
	if err := json.Unmarshal(data, cache); err != nil {
		return nil
	}
	return cache
}

// CachedWorkRepos returns whatever work-repo list is cached (possibly
// stale — matching prefers stale data over none) and whether it is still
// fresh (within the 24h TTL).
func CachedWorkRepos() (repos []WorkRepo, fresh bool) {
	return cachedWorkRepos(loadWorkRepoCache(), time.Now())
}

func cachedWorkRepos(cache *workRepoCache, now time.Time) ([]WorkRepo, bool) {
	if cache == nil {
		return nil, false
	}
	return cache.Repos, now.Sub(cache.FetchedAt) <= workRepoCacheTTL
}

// RefreshWorkRepos fetches the server's work-repo list and stores it in the
// cache.
func RefreshWorkRepos() error {
	client, err := NewClientFromToken()
	if err != nil {
		return err
	}
	repos, err := client.GetWorkRepos()
	if err != nil {
		return err
	}

	path, err := workRepoCachePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(&workRepoCache{Repos: repos, FetchedAt: time.Now()})
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	return os.WriteFile(path, data, 0o644)
}

// RepoIsWork reports whether the repo at slug ("owner/name") counts as a
// work repo: its full name or owner matches the server-provided list, or its
// owner is in the manually configured hookOrgs supplement.
func RepoIsWork(slug string, workRepos []WorkRepo, hookOrgs []string) bool {
	if slug == "" {
		return false
	}
	owner := SlugOwner(slug)
	for _, repo := range workRepos {
		if strings.EqualFold(repo.FullName, slug) {
			return true
		}
		if repo.Owner != "" && strings.EqualFold(repo.Owner, owner) {
			return true
		}
	}
	return OwnerMatchesOrgs(owner, hookOrgs)
}
