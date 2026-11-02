package desktimers

import (
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/config"
)

// ── which GitHub account a repository is asked as ───────────────────────────
// gh can be signed in to several accounts on one host, one of them active.
// A repository one account cannot see is "Not Found" to it, so deskgit asks
// which account a repository uses, once, and keeps the answer in the
// repository's own git config.

// githubAccountKey is the git config key holding the account chosen for a
// repository.
const githubAccountKey = "deskgit.githubAccount"

// GitHubAccounts are the accounts gh is signed in to on one host.
type GitHubAccounts struct {
	// Logins has the active account first, then the others as gh lists them.
	Logins []string
	Active string
	// FromEnv says a token in the environment (GH_TOKEN and the like)
	// decides, as it does for gh, and no account is chosen.
	FromEnv bool
}

var readGhConfig = func() (*config.Config, error) { return config.Read(nil) }

// envGitHubToken is the token the environment gives for host, the way gh
// reads it: GH_TOKEN or GITHUB_TOKEN for github.com, the _ENTERPRISE_ pair
// for any other host.
func envGitHubToken(host string) string {
	names := []string{"GH_TOKEN", "GITHUB_TOKEN"}
	if auth.IsEnterprise(host) {
		names = []string{"GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}
	}
	for _, name := range names {
		if token := os.Getenv(name); token != "" {
			return token
		}
	}
	return ""
}

// ListGitHubAccounts are the accounts gh is signed in to on host.
func ListGitHubAccounts(host string) GitHubAccounts {
	host = auth.NormalizeHostname(host)
	if envGitHubToken(host) != "" {
		return GitHubAccounts{FromEnv: true}
	}
	cfg, err := readGhConfig()
	if err != nil || cfg == nil {
		return GitHubAccounts{}
	}
	active, _ := cfg.Get([]string{"hosts", host, "user"})
	logins := []string{}
	if active != "" {
		logins = append(logins, active)
	}
	others, _ := cfg.Keys([]string{"hosts", host, "users"})
	for _, login := range others {
		if !containsLogin(logins, login) {
			logins = append(logins, login)
		}
	}
	return GitHubAccounts{Logins: logins, Active: active}
}

// containsLogin compares as GitHub does: logins ignore case.
func containsLogin(logins []string, login string) bool {
	return slices.ContainsFunc(logins, func(l string) bool { return strings.EqualFold(l, login) })
}

// ghAccountToken is one account's token, from gh, which keeps it in the
// system keyring.
var ghAccountToken = func(host string, login string) string {
	gh := os.Getenv("GH_PATH")
	if gh == "" {
		gh, _ = exec.LookPath("gh")
	}
	if gh == "" {
		return ""
	}
	out, err := exec.Command(gh, "auth", "token", "--hostname", host, "--user", login).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// activeGitHubToken is the active account's token, as everything outside
// deskgit's account choice gets it.
var activeGitHubToken = func(host string) string {
	token, _ := auth.TokenForHost(host)
	return token
}

// RememberedGitHubAccount is the account chosen for the repository, or "".
func RememberedGitHubAccount(repoPath string) string {
	out, err := gitOutput(repoPath, "config", "--get", githubAccountKey)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// RememberGitHubAccount keeps the account chosen for the repository.
func RememberGitHubAccount(repoPath string, login string) error {
	_, err := gitOutput(repoPath, "config", "--local", githubAccountKey, login)
	return err
}

// NeedsGitHubAccountChoice says gh has more than one account on host and
// none has been chosen for the repository (or the one chosen is signed out).
func NeedsGitHubAccountChoice(repoPath string, host string) bool {
	accounts := ListGitHubAccounts(host)
	if accounts.FromEnv || len(accounts.Logins) < 2 {
		return false
	}
	return !containsLogin(accounts.Logins, RememberedGitHubAccount(repoPath))
}

// GitHubToken is the token the repository is asked with on host, and the
// account it is (login is "" when the environment's token decides): the
// environment's, else the account chosen for the repository, else gh's
// active account.
func GitHubToken(repoPath string, host string) (token string, login string) {
	accounts := ListGitHubAccounts(host)
	if accounts.FromEnv {
		return envGitHubToken(auth.NormalizeHostname(host)), ""
	}
	if chosen := RememberedGitHubAccount(repoPath); chosen != "" && containsLogin(accounts.Logins, chosen) {
		if token := ghAccountToken(host, chosen); token != "" {
			return token, chosen
		}
	}
	return activeGitHubToken(host), accounts.Active
}
