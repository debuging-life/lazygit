package desktimers

import (
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/assert"
)

// fakeGh stands in for gh: its hosts.yml, and a token for each account.
func fakeGh(t *testing.T, hostsYAML string) *[]string {
	t.Helper()
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		t.Setenv(name, "")
	}
	asked := []string{}
	originalConfig, originalToken, originalActive := readGhConfig, ghAccountToken, activeGitHubToken
	readGhConfig = func() (*config.Config, error) { return config.ReadFromString(hostsYAML), nil }
	ghAccountToken = func(host string, login string) string {
		asked = append(asked, host+" "+login)
		if login == "signed-out" {
			return ""
		}
		return "token-of-" + login
	}
	activeGitHubToken = func(host string) string { return "active-token" }
	t.Cleanup(func() {
		readGhConfig, ghAccountToken, activeGitHubToken = originalConfig, originalToken, originalActive
	})
	return &asked
}

const threeAccounts = `
hosts:
    github.com:
        git_protocol: ssh
        users:
            ada:
            bea:
            cy:
        user: bea
`

func TestListGitHubAccountsPutsTheActiveOneFirst(t *testing.T) {
	fakeGh(t, threeAccounts)

	accounts := ListGitHubAccounts("github.com")

	assert.Equal(t, GitHubAccounts{Logins: []string{"bea", "ada", "cy"}, Active: "bea"}, accounts)
	assert.Equal(t, GitHubAccounts{Logins: []string{}}, ListGitHubAccounts("ghe.acme.com"))
}

func TestATokenInTheEnvironmentDecides(t *testing.T) {
	fakeGh(t, threeAccounts)
	t.Setenv("GITHUB_TOKEN", "from-env")
	repo := newCommitRepo(t)
	assert.NoError(t, RememberGitHubAccount(repo, "ada"))

	assert.Equal(t, GitHubAccounts{FromEnv: true}, ListGitHubAccounts("github.com"))
	assert.False(t, NeedsGitHubAccountChoice(repo, "github.com"))
	token, login := GitHubToken(repo, "github.com")
	assert.Equal(t, "from-env", token)
	assert.Equal(t, "", login)
	// github.com's variables are not an enterprise host's.
	assert.False(t, ListGitHubAccounts("ghe.acme.com").FromEnv)
}

func TestAChoiceIsNeededOnlyAmongSeveralAccounts(t *testing.T) {
	repo := newCommitRepo(t)

	fakeGh(t, threeAccounts)
	assert.True(t, NeedsGitHubAccountChoice(repo, "github.com"))

	assert.NoError(t, RememberGitHubAccount(repo, "ADA"))
	assert.False(t, NeedsGitHubAccountChoice(repo, "github.com"))

	// One signed out since is asked about again.
	assert.NoError(t, RememberGitHubAccount(repo, "gone"))
	assert.True(t, NeedsGitHubAccountChoice(repo, "github.com"))

	fakeGh(t, "hosts:\n    github.com:\n        users:\n            ada:\n        user: ada\n")
	assert.False(t, NeedsGitHubAccountChoice(repo, "github.com"))
}

func TestGitHubTokenIsTheChosenAccounts(t *testing.T) {
	asked := fakeGh(t, threeAccounts)
	repo := newCommitRepo(t)

	token, login := GitHubToken(repo, "github.com")
	assert.Equal(t, "active-token", token)
	assert.Equal(t, "bea", login)
	assert.Empty(t, *asked)

	assert.NoError(t, RememberGitHubAccount(repo, "ada"))
	assert.Equal(t, "ada", RememberedGitHubAccount(repo))
	token, login = GitHubToken(repo, "github.com")
	assert.Equal(t, "token-of-ada", token)
	assert.Equal(t, "ada", login)
	assert.Equal(t, []string{"github.com ada"}, *asked)
}

func TestGitHubTokenFallsBackToTheActiveAccount(t *testing.T) {
	fakeGh(t, strings.Replace(threeAccounts, "            cy:\n", "            cy:\n            signed-out:\n", 1))
	repo := newCommitRepo(t)

	// gh lists the account but has no token for it.
	assert.NoError(t, RememberGitHubAccount(repo, "signed-out"))
	token, login := GitHubToken(repo, "github.com")
	assert.Equal(t, "active-token", token)
	assert.Equal(t, "bea", login)

	// Nor is an account gh does not list asked for.
	asked := fakeGh(t, threeAccounts)
	assert.NoError(t, RememberGitHubAccount(repo, "stranger"))
	token, _ = GitHubToken(repo, "github.com")
	assert.Equal(t, "active-token", token)
	assert.Empty(t, *asked)
}

func TestTheChoiceIsTheRepositorysOwn(t *testing.T) {
	fakeGh(t, threeAccounts)
	one, other := newCommitRepo(t), newCommitRepo(t)

	assert.NoError(t, RememberGitHubAccount(one, "ada"))

	assert.Equal(t, "ada", RememberedGitHubAccount(one))
	assert.Equal(t, "", RememberedGitHubAccount(other))
}
