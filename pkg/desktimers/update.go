package desktimers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FormulaURL is the Homebrew tap formula the update check reads the latest
// released version from.
const FormulaURL = "https://raw.githubusercontent.com/debuging-life/homebrew-tap/main/Formula/deskgit.rb"

const (
	updateCheckInterval = 24 * time.Hour
	updateCheckTimeout  = 3 * time.Second
	updateCacheFileName = "update-check.json"
)

var formulaVersionRegex = regexp.MustCompile(`(?m)^\s*version\s+"([^"]+)"`)

// ParseFormulaVersion extracts the `version "X.Y.Z"` value from a Homebrew
// formula, or "" when absent.
func ParseFormulaVersion(formula string) string {
	match := formulaVersionRegex.FindStringSubmatch(formula)
	if match == nil {
		return ""
	}
	return match[1]
}

// parseSemver parses "X.Y.Z" (optionally v-prefixed); ok is false for
// anything else (dev builds like "HEAD-abc123", "unversioned", ...).
func parseSemver(v string) (parts [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	segments := strings.Split(v, ".")
	if len(segments) != 3 {
		return parts, false
	}
	for i, segment := range segments {
		n, err := strconv.Atoi(segment)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// IsNewerVersion reports whether latest is a strictly newer semver than
// current. False when either side isn't a plain semver (dev builds never
// nag).
func IsNewerVersion(latest string, current string) bool {
	l, ok := parseSemver(latest)
	if !ok {
		return false
	}
	c, ok := parseSemver(current)
	if !ok {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

type updateCheckCache struct {
	LatestVersion string    `json:"latestVersion"`
	CheckedAt     time.Time `json:"checkedAt"`
}

func updateCachePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, updateCacheFileName), nil
}

func loadUpdateCache() *updateCheckCache {
	path, err := updateCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	cache := &updateCheckCache{}
	if err := json.Unmarshal(data, cache); err != nil {
		return nil
	}
	return cache
}

func saveUpdateCache(cache *updateCheckCache) {
	path, err := updateCachePath()
	if err != nil {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o644)
}

// cachedLatestVersion returns the cached latest version when the cache is
// fresh (checked within updateCheckInterval).
func cachedLatestVersion(cache *updateCheckCache, now time.Time) (string, bool) {
	if cache == nil || cache.LatestVersion == "" {
		return "", false
	}
	if now.Sub(cache.CheckedAt) > updateCheckInterval {
		return "", false
	}
	return cache.LatestVersion, true
}

// formulaURLOverride lets tests point the fetch at a local server.
var formulaURLOverride string

func formulaURL() string {
	if formulaURLOverride != "" {
		return formulaURLOverride
	}
	return FormulaURL
}

// LatestReleasedVersion returns the latest released deskgit version from the
// Homebrew tap, hitting the network at most once per 24h (cached in the
// config dir). force bypasses the cache.
func LatestReleasedVersion(force bool) (string, error) {
	if !force {
		if latest, ok := cachedLatestVersion(loadUpdateCache(), time.Now()); ok {
			return latest, nil
		}
	}

	httpClient := &http.Client{Timeout: updateCheckTimeout}
	resp, err := httpClient.Get(formulaURL())
	if err != nil {
		return "", fmt.Errorf("fetching formula: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching formula: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading formula: %w", err)
	}

	latest := ParseFormulaVersion(string(body))
	if latest == "" {
		return "", fmt.Errorf("no version found in formula")
	}

	saveUpdateCache(&updateCheckCache{LatestVersion: latest, CheckedAt: time.Now()})
	return latest, nil
}

// CheckForUpdate reports whether a newer release than currentVersion exists.
// Dev builds (non-semver versions) never report an update.
func CheckForUpdate(currentVersion string) (latest string, newer bool, err error) {
	if _, ok := parseSemver(currentVersion); !ok {
		return "", false, nil
	}
	latest, err = LatestReleasedVersion(false)
	if err != nil {
		return "", false, err
	}
	return latest, IsNewerVersion(latest, currentVersion), nil
}
