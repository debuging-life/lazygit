package desktimers

import (
	"os/exec"
	"regexp"
	"strings"
)

var remoteURLSlugRegex = regexp.MustCompile(`[:/]([^:/]+/[^:/]+?)(?:\.git)?/?$`)

func remoteURLToSlug(url string) string {
	match := remoteURLSlugRegex.FindStringSubmatch(strings.TrimSpace(url))
	if match == nil {
		return ""
	}
	return match[1]
}

// SlugOwner returns the owner part of an "owner/name" slug, or "".
func SlugOwner(slug string) string {
	owner, _, found := strings.Cut(slug, "/")
	if !found {
		return ""
	}
	return owner
}

// OwnerMatchesOrgs reports whether owner (case-insensitively) is one of the
// configured "work" orgs. An empty owner never matches.
func OwnerMatchesOrgs(owner string, orgs []string) bool {
	if owner == "" {
		return false
	}
	for _, org := range orgs {
		if strings.EqualFold(owner, org) {
			return true
		}
	}
	return false
}

// RepoSlug returns the origin remote's "owner/name" for the repo at
// repoPath, or "" when there is no origin or it can't be parsed.
func RepoSlug(repoPath string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return remoteURLToSlug(string(out))
}

// CurrentBranch returns the checked-out branch name for the repo at
// repoPath, or "" (detached HEAD, errors).
func CurrentBranch(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" { // detached
		return ""
	}
	return branch
}
