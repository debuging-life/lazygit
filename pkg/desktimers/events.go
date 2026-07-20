package desktimers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// reportTimeout bounds the fire-and-forget event report; the UI never waits
// on it, but the goroutine shouldn't linger either.
const reportTimeout = 3 * time.Second

// ReportTaskSelected reports a task pick to DeskTimers
// (POST /api/git-client/events, type "task_selected"). Non-critical: callers
// fire it in the background and only log failures.
func (c *Client) ReportTaskSelected(code string, repo string, branch string) error {
	body, err := json.Marshal(map[string]string{
		"type":       "task_selected",
		"code":       code,
		"repo":       repo,
		"branch":     branch,
		"occurredAt": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/git-client/events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building event request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("reporting event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reporting event: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// remoteURLToSlug extracts "owner/name" from common git remote URL shapes
// (ssh, ssh-alias, https), or "" when it doesn't look like one.
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
