package desktimers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkRepo is a git repo connected to one of the user's DeskTimers
// workspaces; deskgit auto-installs hooks in these.
type WorkRepo struct {
	FullName string `json:"fullName"`
	Owner    string `json:"owner"`
}

type workReposResponse struct {
	Success bool       `json:"success"`
	Data    []WorkRepo `json:"data"`
}

// GetWorkRepos fetches all repos connected across the user's workspaces.
func (c *Client) GetWorkRepos() ([]WorkRepo, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/git-client/repos", nil)
	if err != nil {
		return nil, fmt.Errorf("building repos request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching repos: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching repos: unexpected status %d", resp.StatusCode)
	}
	parsed := workReposResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing repos response: %w", err)
	}
	return parsed.Data, nil
}

// ─── Work-repo cache (config dir, 24h TTL) ──────────────────────────────────

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
