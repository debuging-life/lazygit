package hosting_service

import (
	"os"
	"testing"

	"github.com/jesseduffield/lazygit/pkg/fakes"
	"github.com/jesseduffield/lazygit/pkg/i18n"
	"github.com/stretchr/testify/assert"
)

// TestMain keeps every test here off the machine's own ~/.ssh/config: no
// name is an alias unless a test says so.
func TestMain(m *testing.M) {
	resolveSSHHost = func(alias string) string { return alias }
	os.Exit(m.Run())
}

func stubSSHAliases(t *testing.T, aliases map[string]string) {
	t.Helper()
	original := resolveSSHHost
	resolveSSHHost = func(alias string) string {
		if host, ok := aliases[alias]; ok {
			return host
		}
		return alias
	}
	t.Cleanup(func() { resolveSSHHost = original })
}

func TestGetServiceInfoThroughAnSSHAlias(t *testing.T) {
	stubSSHAliases(t, map[string]string{"github-work": "github.com"})
	mgr := NewHostingServiceMgr(&fakes.FakeFieldLogger{}, i18n.EnglishTranslationSet(),
		"git@github-work:acme/clock.git", nil)

	info, err := mgr.GetServiceInfo()

	problem := ""
	if err != nil {
		problem = err.Error()
	}
	assert.Equal(t, ServiceInfo{
		Provider: "github", WebDomain: "github.com", Owner: "acme", Repository: "clock", RepoName: "acme/clock",
	}, info)
	assert.Empty(t, problem)
}

func TestWithSSHAlias(t *testing.T) {
	asked := []string{}
	original := resolveSSHHost
	resolveSSHHost = func(alias string) string {
		asked = append(asked, alias)
		return map[string]string{
			"github-work": "github.com",
			"gh-443":      "SSH.GitHub.com",
			"acme":        "git.acme.com",
			"box":         "box.example.com",
		}[alias]
	}
	t.Cleanup(func() { resolveSSHHost = original })

	enterprise := map[string]string{"git.acme.com": "github:ghe.acme.com"}
	for _, s := range []struct {
		remote   string
		services map[string]string
		expected map[string]string
	}{
		{"git@github-work:a/b.git", nil, map[string]string{"github-work": "github:github.com"}},
		{"ssh://git@github-work:22/a/b.git", nil, map[string]string{"github-work": "github:github.com"}},
		{"git@gh-443:a/b.git", nil, map[string]string{"gh-443": "github:github.com"}},
		// An alias for a host configured by hand takes that host's service.
		{"git@acme:a/b.git", enterprise, map[string]string{"git.acme.com": "github:ghe.acme.com", "acme": "github:ghe.acme.com"}},
		// A host that is no service is left alone.
		{"git@box:a/b.git", nil, nil},
		// So is a name ssh does not know.
		{"git@nowhere:a/b.git", nil, nil},
	} {
		assert.Equal(t, s.expected, withSSHAlias(s.remote, s.services), s.remote)
	}
	// The services passed in are never changed.
	assert.Equal(t, map[string]string{"git.acme.com": "github:ghe.acme.com"}, enterprise)

	asked = []string{}
	byHand := map[string]string{"github-work": "gitlab:gitlab.example.com"}
	assert.Equal(t, byHand, withSSHAlias("git@github-work:a/b.git", byHand))
	assert.Equal(t, enterprise, withSSHAlias("git@git.acme.com:a/b.git", enterprise))
	assert.Nil(t, withSSHAlias("git@github.com:a/b.git", nil))
	assert.Nil(t, withSSHAlias("https://github-work/a/b.git", nil))
	// None of these needed ssh.
	assert.Empty(t, asked)
}
