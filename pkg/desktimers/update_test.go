package desktimers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const sampleFormula = `class Deskgit < Formula
  desc "DeskTimers git client"
  homepage "https://desktimers.com"
  version "0.3.1"
  url "https://github.com/debuging-life/deskgit/releases/download/v0.3.1/deskgit_0.3.1_darwin_arm64.tar.gz"
end`

func TestParseFormulaVersion(t *testing.T) {
	if got := ParseFormulaVersion(sampleFormula); got != "0.3.1" {
		t.Errorf("ParseFormulaVersion = %q, want 0.3.1", got)
	}
	if got := ParseFormulaVersion("class X < Formula\nend"); got != "" {
		t.Errorf("formula without version should yield \"\", got %q", got)
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"0.3.1", "0.3.0", true},
		{"0.3.1", "0.3.1", false},
		{"0.3.0", "0.3.1", false},
		{"1.0.0", "0.9.9", true},
		{"0.10.0", "0.9.0", true}, // numeric, not lexicographic
		{"v0.3.1", "v0.3.0", true},
		{"0.3.1", "HEAD-abc123", false},   // dev build never nags
		{"0.3.1", "unversioned", false},   // dev build never nags
		{"not-a-version", "0.3.0", false}, // garbage latest
	}
	for _, tt := range tests {
		if got := IsNewerVersion(tt.latest, tt.current); got != tt.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestCachedLatestVersion(t *testing.T) {
	now := time.Now()

	if _, ok := cachedLatestVersion(nil, now); ok {
		t.Error("nil cache must miss")
	}
	fresh := &updateCheckCache{LatestVersion: "0.3.1", CheckedAt: now.Add(-time.Hour)}
	if latest, ok := cachedLatestVersion(fresh, now); !ok || latest != "0.3.1" {
		t.Errorf("fresh cache should hit, got %q/%v", latest, ok)
	}
	stale := &updateCheckCache{LatestVersion: "0.3.1", CheckedAt: now.Add(-25 * time.Hour)}
	if _, ok := cachedLatestVersion(stale, now); ok {
		t.Error("cache older than 24h must miss")
	}
}

func TestCheckForUpdate(t *testing.T) {
	setupConfigDir(t) // isolated cache dir

	fetches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		w.Write([]byte(sampleFormula))
	}))
	defer server.Close()
	formulaURLOverride = server.URL
	t.Cleanup(func() { formulaURLOverride = "" })

	latest, newer, err := CheckForUpdate("0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if latest != "0.3.1" || !newer {
		t.Errorf("got latest=%q newer=%v, want 0.3.1/true", latest, newer)
	}

	// Second check within 24h hits the cache, not the network.
	if _, _, err := CheckForUpdate("0.3.0"); err != nil {
		t.Fatal(err)
	}
	if fetches != 1 {
		t.Errorf("expected 1 network fetch (cache on second), got %d", fetches)
	}

	// Dev build: no fetch, no update.
	latest, newer, err = CheckForUpdate("HEAD-abc123")
	if err != nil || latest != "" || newer {
		t.Errorf("dev build should skip the check, got %q/%v/%v", latest, newer, err)
	}

	// Up to date.
	_, newer, err = CheckForUpdate("0.3.1")
	if err != nil {
		t.Fatal(err)
	}
	if newer {
		t.Error("same version should not report an update")
	}
}
