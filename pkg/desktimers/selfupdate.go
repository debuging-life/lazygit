package desktimers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

// ReleasesBaseURL is the R2 public bucket root. Override via DESKGIT_RELEASES_URL.
const ReleasesBaseURL = "https://releases.desktimers.com/deskgit"

// manifestTimeout is how long we wait for the manifest fetch.
const manifestTimeout = 10 * time.Second

// downloadTimeout is how long we wait for a binary download.
const downloadTimeout = 5 * time.Minute

// ReleaseAsset describes a single platform binary in the release manifest.
type ReleaseAsset struct {
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	HookURL    string `json:"hook_url"`
	HookSHA256 string `json:"hook_sha256"`
}

// ReleaseManifest is the JSON structure at <base>/latest.json.
type ReleaseManifest struct {
	Version     string                  `json:"version"`
	PublishedAt string                  `json:"published_at"`
	Assets      map[string]ReleaseAsset `json:"assets"` // key: "darwin/arm64", "linux/amd64", …
}

// releasesBaseURL returns the effective base URL (env override or constant).
func releasesBaseURL() string {
	if v := os.Getenv("DESKGIT_RELEASES_URL"); v != "" {
		return v
	}
	return ReleasesBaseURL
}

// ManifestURL returns the URL of the latest.json manifest.
func ManifestURL() string {
	return releasesBaseURL() + "/latest.json"
}

// FetchManifest downloads and parses the release manifest.
func FetchManifest() (*ReleaseManifest, error) {
	client := &http.Client{Timeout: manifestTimeout}
	resp, err := client.Get(ManifestURL())
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching manifest: status %d", resp.StatusCode)
	}
	var m ReleaseManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}

// PlatformKey returns the manifest asset key for the current OS/arch,
// e.g. "darwin/arm64".
func PlatformKey() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// DownloadAndVerify downloads url to a temporary file, verifies its SHA256,
// and returns the temp file path. The caller is responsible for removing it.
func DownloadAndVerify(url, expectedSHA256 string) (string, error) {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: status %d", url, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "deskgit-update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing download: %w", err)
	}
	tmp.Close()

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedSHA256 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("checksum mismatch: got %s want %s", got, expectedSHA256)
	}

	return tmpPath, nil
}

// ReplaceExecutable atomically replaces dest with the file at src, preserving
// the executable bit. src is removed on success.
func ReplaceExecutable(src, dest string) error {
	if err := os.Chmod(src, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	// Rename is atomic on the same filesystem; fall back to copy+rename if
	// src and dest are on different filesystems.
	if err := os.Rename(src, dest); err != nil {
		if err2 := copyFile(src, dest); err2 != nil {
			return fmt.Errorf("replacing binary: %w", err2)
		}
		os.Remove(src)
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	return os.Rename(tmp, dest)
}
